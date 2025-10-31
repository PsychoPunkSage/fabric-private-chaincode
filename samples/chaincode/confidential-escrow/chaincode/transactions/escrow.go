package transactions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hyperledger-labs/cc-tools/accesscontrol"
	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	"github.com/hyperledger-labs/cc-tools/transactions"
)

var CreateAndLockEscrow = transactions.Transaction{
	Tag:         "createAndLockEscrow",
	Label:       "Create and Lock Escrow",
	Description: "Creates a new escrow and immediately locks funds",
	Method:      "POST",
	Callers: []accesscontrol.Caller{
		{MSP: "Org1MSP", OU: "admin"},
		{MSP: "Org2MSP", OU: "admin"},
	},
	Args: []transactions.Argument{
		{Tag: "escrowId", Label: "Escrow ID", DataType: "string", Required: true},
		{Tag: "buyerPubKey", Label: "Buyer Public Key", DataType: "string", Required: true},
		{Tag: "sellerPubKey", Label: "Seller Public Key", DataType: "string", Required: true},
		{Tag: "amount", Label: "Escrowed Amount", DataType: "number", Required: true},
		{Tag: "assetType", Label: "Asset Type Reference", DataType: "->digitalAsset", Required: true},
		{Tag: "parcelId", Label: "Parcel ID", DataType: "string", Required: true},
		{Tag: "secret", Label: "Secret Key", DataType: "string", Required: true},
		{Tag: "buyerCertHash", Label: "buyer Certificate Hash", DataType: "string", Required: true},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]any) ([]byte, errors.ICCError) {
		escrowId, _ := req["escrowId"].(string)
		buyerPubKey, _ := req["buyerPubKey"].(string)
		sellerPubKey, _ := req["sellerPubKey"].(string)
		amount, _ := req["amount"].(float64)
		assetType, _ := req["assetType"].(any)
		parcelId, _ := req["parcelId"].(string)
		secret, _ := req["secret"].(string)
		buyerCertHash, _ := req["buyerCertHash"].(string)

		// Extract assetId from assetType reference
		var assetId string
		assetKey, ok := assetType.(assets.Key)
		if !ok {
			return nil, errors.NewCCError(fmt.Sprintf("Invalid assetType: expected map, got %T", assetType), 400)
		}

		keyStr, exists := assetKey["@key"]
		if !exists {
			return nil, errors.NewCCError("Invalid assetType: @key field not found", 400)
		}

		keyString, ok := keyStr.(string)
		if !ok {
			return nil, errors.NewCCError(fmt.Sprintf("Invalid assetType: @key is not string, got %T", assetKey), 400)
		}

		parts := strings.Split(keyString, ":")
		if len(parts) != 2 {
			return nil, errors.NewCCError("Invalid assetType: @key format incorrect", 400)
		}
		assetId = parts[1]

		// 0. Check for wallet existence
		hash := sha256.Sum256([]byte(sellerPubKey))
		sellerPubKeyHash := hex.EncodeToString(hash[:])

		fmt.Printf("DEBUG: Seller PubKey: %s\n", sellerPubKey)
		fmt.Printf("DEBUG: Seller PubKey Hash: %s\n", sellerPubKeyHash)

		sellerUserDirKey, err := assets.NewKey(map[string]any{
			"@assetType":    "userdir",
			"publicKeyHash": sellerPubKeyHash,
		})
		if err != nil {
			return nil, errors.NewCCError(fmt.Sprintf("Seller's Key cannot be found from user dir: %v", err), 404)
		}

		sellerUserDir, err := sellerUserDirKey.Get(stub)
		if err != nil {
			return nil, errors.NewCCError(fmt.Sprintf("Seller wallet not found. Seller must create wallet first. Details: %v", err), 404)
		}
		fmt.Printf("DEBUG: Seller UserDir found: %+v\n", sellerUserDir)
		sellerWalletUUID := sellerUserDir.GetProp("walletUUID").(string)
		fmt.Printf("DEBUG: Seller WalletID: %s\n", sellerWalletUUID)

		// Lookup buyer wallet using publicKeyHash property
		hash = sha256.Sum256([]byte(buyerPubKey))
		buyerPubKeyHash := hex.EncodeToString(hash[:])

		buyerUserDirKey, err := assets.NewKey(map[string]any{
			"@assetType":    "userdir",
			"publicKeyHash": buyerPubKeyHash,
		})
		if err != nil {
			return nil, errors.NewCCError(fmt.Sprintf("Seller's Key cannot be found from user dir: %v", err), 404)
		}

		buyerUserDir, err := buyerUserDirKey.Get(stub)
		if err != nil {
			return nil, errors.NewCCError("Buyer wallet not found. Buyer must create wallet first.", 404)
		}
		buyerWalletUUID := buyerUserDir.GetProp("walletUUID").(string)

		// 1. Get and verify buyer wallet ownership
		buyerWalletKey := assets.Key{"@key": "wallet:" + buyerWalletUUID}
		buyerWallet, err := buyerWalletKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading buyer wallet", err.Status())
		}

		if buyerWallet.GetProp("ownerCertHash").(string) != buyerCertHash {
			return nil, errors.NewCCError("Unauthorized: Certificate hash mismatch", 403)
		}

		// 2. Get wallet balances
		digitalAssetTypes := buyerWallet.GetProp("digitalAssetTypes").([]any)
		balances := buyerWallet.GetProp("balances").([]any)

		var escrowBalances []any
		if buyerWallet.GetProp("escrowBalances") != nil {
			escrowBalances = buyerWallet.GetProp("escrowBalances").([]any)
		} else {
			escrowBalances = make([]any, len(balances))
			for i := range escrowBalances {
				escrowBalances[i] = 0.0
			}
		}

		// 3. Find asset index and check sufficient balance
		assetFound := false
		assetIndex := -1
		for i, assetRef := range digitalAssetTypes {
			var refAssetId string
			switch ref := assetRef.(type) {
			case map[string]any:
				refAssetId = strings.Split(ref["@key"].(string), ":")[1]
			case string:
				refAssetId = ref
			}

			if refAssetId == assetId {
				currentBalance := balances[i].(float64)
				if currentBalance < amount {
					return nil, errors.NewCCError("Insufficient balance", 400)
				}
				assetFound = true
				assetIndex = i
				break
			}
		}

		if !assetFound {
			return nil, errors.NewCCError("Asset not found in wallet", 404)
		}

		// 4. Move funds from balances to escrowBalances
		currentBalance := balances[assetIndex].(float64)
		currentEscrowBalance := escrowBalances[assetIndex].(float64)

		balances[assetIndex] = currentBalance - amount
		escrowBalances[assetIndex] = currentEscrowBalance + amount

		// 5. Update wallet
		buyerWalletUpdate := map[string]any{
			"balances":          balances,
			"escrowBalances":    escrowBalances,
			"digitalAssetTypes": digitalAssetTypes,
		}
		_, err = buyerWallet.Update(stub, buyerWalletUpdate)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving updated wallet", err.Status())
		}

		// Compute condition hash: SHA256(secret + parcelId)
		conditionData := secret + parcelId
		conditionHash := sha256.Sum256([]byte(conditionData))
		conditionValue := hex.EncodeToString(conditionHash[:])

		// 6. Create escrow with "Active" status
		escrowMap := make(map[string]any)
		escrowMap["@assetType"] = "escrow"
		escrowMap["escrowId"] = escrowId
		escrowMap["buyerPubKey"] = buyerPubKey
		escrowMap["sellerPubKey"] = sellerPubKey
		escrowMap["buyerWalletUUID"] = buyerWalletUUID
		escrowMap["sellerWalletUUID"] = sellerWalletUUID
		escrowMap["parcelId"] = parcelId
		escrowMap["amount"] = amount
		escrowMap["assetType"] = assetType
		escrowMap["conditionValue"] = conditionValue
		escrowMap["status"] = "Active"
		escrowMap["createdAt"] = time.Now()
		escrowMap["buyerCertHash"] = buyerCertHash

		escrowAsset, err := assets.NewAsset(escrowMap)
		if err != nil {
			return nil, errors.WrapError(err, "Failed to create escrow asset")
		}

		_, err = escrowAsset.PutNew(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving escrow on blockchain", err.Status())
		}

		assetJSON, nerr := json.Marshal(escrowAsset)
		if nerr != nil {
			return nil, errors.WrapError(nil, "failed to encode escrow to JSON format")
		}

		return assetJSON, nil
	},
}

// Add VerifyEscrowCondition transaction
var VerifyEscrowCondition = transactions.Transaction{
	Tag: "verifyEscrowCondition",
	Args: []transactions.Argument{
		{Tag: "escrowId", DataType: "string", Required: true},
		{Tag: "secret", DataType: "string", Required: true},
		{Tag: "parcelId", DataType: "string", Required: true},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]any) ([]byte, errors.ICCError) {
		escrowId, _ := req["escrowId"].(string)
		secret, _ := req["secret"].(string)
		parcelId, _ := req["parcelId"].(string)

		// 1. Get escrow by ID
		escrowKey := assets.Key{"@key": "escrow:" + escrowId}
		escrowAsset, err := escrowKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading escrow", err.Status())
		}

		// Check escrow status
		currentStatus := escrowAsset.GetProp("status").(string)
		if currentStatus != "Active" {
			return nil, errors.NewCCError("Escrow is not active", 400)
		}

		// 2. Get stored condition value from escrow
		storedCondition := escrowAsset.GetProp("conditionValue").(string)

		// 3. Compute SHA256(secret + parcelId)
		hasher := sha256.New()
		hasher.Write([]byte(secret + parcelId))
		computedHash := hex.EncodeToString(hasher.Sum(nil))

		// 4. Verify condition: sha256(secret + parcelID) == stored condition
		if computedHash != storedCondition {
			return nil, errors.NewCCError("Condition verification failed: hash mismatch", 403)
		}

		// 5. Update escrow status to "ReadyForRelease"
		escrowUpdate := map[string]any{
			"status": "ReadyForRelease",
		}
		_, err = escrowAsset.Update(stub, escrowUpdate)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving updated escrow", err.Status())
		}

		// 6. Return success response
		response := map[string]any{
			"message":      "Condition verified successfully",
			"escrowId":     escrowId,
			"status":       "ReadyForRelease",
			"parcelId":     parcelId,
			"computedHash": computedHash,
		}

		responseJSON, jsonErr := json.Marshal(response)
		if jsonErr != nil {
			return nil, errors.WrapError(nil, "failed to encode response to JSON format")
		}

		return responseJSON, nil
	},
}

var ReleaseEscrow = transactions.Transaction{
	Tag:         "releaseEscrow",
	Label:       "Release Escrow",
	Description: "Seller releases escrow with secret and parcelId",
	Method:      "POST",
	Callers: []accesscontrol.Caller{
		{MSP: "Org1MSP", OU: "admin"},
		{MSP: "Org2MSP", OU: "admin"},
	},
	Args: []transactions.Argument{
		{Tag: "escrowUUID", DataType: "string", Required: true},
		{Tag: "secret", DataType: "string", Required: true},
		{Tag: "parcelId", DataType: "string", Required: true},
		{Tag: "sellerCertHash", DataType: "string", Required: true},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]any) ([]byte, errors.ICCError) {
		escrowUUID, _ := req["escrowUUID"].(string)
		secret, _ := req["secret"].(string)
		parcelId, _ := req["parcelId"].(string)
		sellerCertHash, _ := req["sellerCertHash"].(string)

		// Get escrow
		escrowKey := assets.Key{"@key": "escrow:" + escrowUUID}
		escrowAsset, err := escrowKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Escrow not found", 404)
		}

		// Verify status
		if escrowAsset.GetProp("status").(string) != "Active" {
			return nil, errors.NewCCError("Escrow is not active", 400)
		}

		// Verify parcelId matches
		if escrowAsset.GetProp("parcelId").(string) != parcelId {
			return nil, errors.NewCCError("Invalid parcel ID", 403)
		}

		// Verify condition: SHA256(secret + parcelId)
		conditionData := secret + parcelId
		computedHash := sha256.Sum256([]byte(conditionData))
		computedCondition := hex.EncodeToString(computedHash[:])

		storedCondition := escrowAsset.GetProp("conditionValue").(string)
		if computedCondition != storedCondition {
			return nil, errors.NewCCError("Invalid secret", 403)
		}

		// Get seller wallet
		sellerWalletId := escrowAsset.GetProp("sellerWalletUUID").(string)
		sellerWalletKey := assets.Key{"@key": "wallet:" + sellerWalletId}
		sellerWallet, err := sellerWalletKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Seller wallet not found", 404)
		}

		// Verify seller authorization
		if sellerWallet.GetProp("ownerCertHash").(string) != sellerCertHash {
			return nil, errors.NewCCError("Unauthorized: Not the seller", 403)
		}

		// Get buyer wallet
		buyerWalletId := escrowAsset.GetProp("buyerWalletUUID").(string)
		buyerWalletKey := assets.Key{"@key": "wallet:" + buyerWalletId}
		buyerWallet, err := buyerWalletKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Buyer wallet not found", 404)
		}

		// Get asset info
		assetType := escrowAsset.GetProp("assetType").(map[string]any)
		assetId := strings.Split(assetType["@key"].(string), ":")[1]
		amount := escrowAsset.GetProp("amount").(float64)

		// Find asset index in both wallets
		buyerAssets := buyerWallet.GetProp("digitalAssetTypes").([]any)
		buyerBalances := buyerWallet.GetProp("balances").([]any)
		buyerEscrowBalances := buyerWallet.GetProp("escrowBalances").([]any)

		sellerAssets := sellerWallet.GetProp("digitalAssetTypes").([]any)
		sellerBalances := sellerWallet.GetProp("balances").([]any)

		var sellerEscrowBalances []any
		if sellerWallet.GetProp("escrowBalances") != nil {
			sellerEscrowBalances = sellerWallet.GetProp("escrowBalances").([]any)
		} else {
			sellerEscrowBalances = make([]any, len(sellerBalances))
			for i := range sellerEscrowBalances {
				sellerEscrowBalances[i] = 0.0
			}
		}

		var buyerAssetIndex, sellerAssetIndex int = -1, -1

		// Find buyer asset index
		for i, assetRef := range buyerAssets {
			refAssetId := strings.Split(assetRef.(map[string]any)["@key"].(string), ":")[1]
			if refAssetId == assetId {
				buyerAssetIndex = i
				break
			}
		}

		// Find seller asset index
		for i, assetRef := range sellerAssets {
			refAssetId := strings.Split(assetRef.(map[string]any)["@key"].(string), ":")[1]
			if refAssetId == assetId {
				sellerAssetIndex = i
				break
			}
		}

		if sellerAssetIndex == -1 {
			sellerAssets = append(sellerAssets, assetType)
			sellerBalances = append(sellerBalances, 0.0)
			sellerEscrowBalances = append(sellerEscrowBalances, 0.0)
			sellerAssetIndex = len(sellerAssets) - 1
		}

		// Transfer: Reduce buyer escrow balance, increase seller balance
		buyerEscrowBalances[buyerAssetIndex] = buyerEscrowBalances[buyerAssetIndex].(float64) - amount
		sellerBalances[sellerAssetIndex] = sellerBalances[sellerAssetIndex].(float64) + amount

		// Update buyer wallet
		walletUpdate := map[string]any{
			"balances":          buyerBalances,
			"escrowBalances":    buyerEscrowBalances,
			"digitalAssetTypes": buyerAssets,
		}
		_, err = buyerWallet.Update(stub, walletUpdate)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Failed to save buyer wallet", err.Status())
		}

		// Update seller wallet
		walletUpdate = map[string]any{
			"balances":          sellerBalances,
			"escrowBalances":    sellerEscrowBalances,
			"digitalAssetTypes": sellerAssets,
		}
		_, err = sellerWallet.Update(stub, walletUpdate)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Failed to save seller wallet", err.Status())
		}

		// Update escrow status to Released
		escrowUpdate := map[string]any{
			"status": "Released",
		}
		_, err = escrowAsset.Update(stub, escrowUpdate)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Failed to save escrow", err.Status())
		}

		response := map[string]any{
			"message":        "Escrow released successfully",
			"escrowId":       escrowUUID,
			"amount":         amount,
			"sellerWalletId": sellerWalletId,
		}

		responseJSON, _ := json.Marshal(response)
		return responseJSON, nil
	},
}

var RefundEscrow = transactions.Transaction{
	Tag:         "refundEscrow",
	Label:       "Refund Escrow",
	Description: "Buyer refunds escrow if condition not met",
	Method:      "POST",
	Callers: []accesscontrol.Caller{
		{MSP: "Org1MSP", OU: "admin"},
		{MSP: "Org2MSP", OU: "admin"},
	},
	Args: []transactions.Argument{
		{Tag: "escrowUUID", DataType: "string", Required: true},
		// {Tag: "buyerWalletUUID", DataType: "string", Required: true},
		{Tag: "buyerPubKey", DataType: "string", Required: true},
		{Tag: "buyerCertHash", DataType: "string", Required: true},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]any) ([]byte, errors.ICCError) {
		escrowUUID, _ := req["escrowUUID"].(string)
		// buyerWalletUUID, _ := req["buyerWalletUUID"].(string)
		buyerPubKey, _ := req["buyerPubKey"].(string)
		buyerCertHash, _ := req["buyerCertHash"].(string)

		// Get escrow
		escrowKey := assets.Key{"@key": "escrow:" + escrowUUID}
		escrowAsset, err := escrowKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Escrow not found", 404)
		}

		// Get Buyer Wallet
		hash := sha256.Sum256([]byte(buyerPubKey))
		buyerPubKeyHash := hex.EncodeToString(hash[:])

		buyerUserDirKey, err := assets.NewKey(map[string]any{
			"@assetType":    "userdir",
			"publicKeyHash": buyerPubKeyHash,
		})
		if err != nil {
			return nil, errors.NewCCError(fmt.Sprintf("Seller's Key cannot be found from user dir: %v", err), 404)
		}

		buyerUserDir, err := buyerUserDirKey.Get(stub)
		if err != nil {
			return nil, errors.NewCCError("Buyer wallet not found. Buyer must create wallet first.", 404)
		}
		buyerWalletUUID := buyerUserDir.GetProp("walletUUID").(string)

		// Verify status
		if escrowAsset.GetProp("status").(string) != "Active" {
			return nil, errors.NewCCError("Escrow is not active", 400)
		}

		buyerWalletKey := assets.Key{"@key": "wallet:" + buyerWalletUUID} // CHANGED
		buyerWallet, err := buyerWalletKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Buyer wallet not found", 404)
		}
		if buyerWallet.GetProp("ownerCertHash").(string) != buyerCertHash {
			return nil, errors.NewCCError("Unauthorized: Not the buyer", 403)
		}

		// Get asset info
		assetType := escrowAsset.GetProp("assetType").(map[string]any)
		assetId := strings.Split(assetType["@key"].(string), ":")[1]
		amount := escrowAsset.GetProp("amount").(float64)

		// Find asset index
		buyerAssets := buyerWallet.GetProp("digitalAssetTypes").([]any)
		buyerBalances := buyerWallet.GetProp("balances").([]any)
		buyerEscrowBalances := buyerWallet.GetProp("escrowBalances").([]any)

		var buyerAssetIndex int = -1
		for i, assetRef := range buyerAssets {
			refAssetId := strings.Split(assetRef.(map[string]any)["@key"].(string), ":")[1]
			if refAssetId == assetId {
				buyerAssetIndex = i
				break
			}
		}

		if buyerAssetIndex == -1 {
			return nil, errors.NewCCError("Asset not found in wallet", 404)
		}

		// Refund: Move from escrow back to available balance
		buyerEscrowBalances[buyerAssetIndex] = buyerEscrowBalances[buyerAssetIndex].(float64) - amount
		buyerBalances[buyerAssetIndex] = buyerBalances[buyerAssetIndex].(float64) + amount

		// Update buyer wallet
		walletUpdate := map[string]any{
			"balances":          buyerBalances,
			"escrowBalances":    buyerEscrowBalances,
			"digitalAssetTypes": buyerAssets,
		}
		_, err = buyerWallet.Update(stub, walletUpdate)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Failed to save buyer wallet", err.Status())
		}

		// Update escrow status to Refunded
		escrowUpdate := map[string]any{
			"status": "Refunded",
		}
		_, err = escrowAsset.Update(stub, escrowUpdate)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Failed to save escrow", err.Status())
		}

		response := map[string]any{
			"message":         "Escrow refunded successfully",
			"escrowUUID":      escrowUUID,
			"amount":          amount,
			"buyerWalletUUID": buyerWalletUUID,
		}

		responseJSON, _ := json.Marshal(response)
		return responseJSON, nil
	},
}

var ReadEscrow = transactions.Transaction{
	Tag:         "readEscrow",
	Label:       "Read Escrow",
	Description: "Read an Escrow by its escrowId",
	Method:      "GET",
	Callers: []accesscontrol.Caller{
		{
			MSP: "Org1MSP",
			OU:  "admin",
		},
		{
			MSP: "Org2MSP",
			OU:  "admin",
		},
	},

	Args: []transactions.Argument{
		{
			Tag:         "uuid",
			Label:       "UUID",
			Description: "UUID of the Digital Asset to read",
			DataType:    "string",
			Required:    true,
		},
	},

	Routine: func(stub *sw.StubWrapper, req map[string]any) ([]byte, errors.ICCError) {
		uuid, _ := req["uuid"].(string)

		key := assets.Key{
			"@key": "escrow:" + uuid,
		}

		asset, err := key.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading escrow from blockchain", err.Status())
		}

		assetJSON, nerr := json.Marshal(asset)
		if nerr != nil {
			return nil, errors.WrapError(nil, "failed to encode escrow to JSON format")
		}

		return assetJSON, nil
	},
}
