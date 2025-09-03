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

var CreateEscrow = transactions.Transaction{
	Tag:         "createEscrow",
	Label:       "Escrow Creation",
	Description: "Creates a new escrow",
	Method:      "POST",
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
			Tag:         "escrowId",
			Label:       "Escrow ID",
			Description: "ID of Escrow",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:      "buyerPubKey",
			Label:    "Buyer Public Key",
			DataType: "string",
			Required: true,
		},
		{
			Tag:      "sellerPubKey",
			Label:    "Seller Public Key",
			DataType: "string",
			Required: true,
		},
		{
			Tag:      "amount",
			Label:    "Escrowed Amount",
			DataType: "number",
			Required: true,
		},
		{
			Tag:      "assetType",
			Label:    "Asset Type Reference",
			DataType: "->digitalAsset",
			Required: true,
		},
		{
			Tag:      "conditionValue",
			Label:    "Condition Value",
			DataType: "string",
			Required: true,
		},
		{
			Tag:      "status",
			Label:    "Escrow Status",
			DataType: "string",
			Required: true,
		},
		{
			Tag:      "createdAt",
			Label:    "Creation Timestamp",
			DataType: "datetime",
			Required: false,
		},
		{
			Tag:      "buyerCertHash",
			Label:    "Buyer Certificate Hash",
			DataType: "string",
			Required: true,
		},
	},

	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		escrowId, _ := req["escrowId"].(string)
		buyerPubKey, _ := req["buyerPubKey"].(string)
		sellerPubKey, _ := req["sellerPubKey"].(string)
		amount, _ := req["amount"].(float64)
		assetType, _ := req["assetType"].(interface{})
		conditionValue, _ := req["conditionValue"].(string)
		status, _ := req["status"].(string)
		buyerCertHash, _ := req["buyerCertHash"].(string)

		escrowMap := make(map[string]interface{})
		escrowMap["@assetType"] = "escrow"
		escrowMap["escrowId"] = escrowId
		escrowMap["buyerPubKey"] = buyerPubKey
		escrowMap["sellerPubKey"] = sellerPubKey
		escrowMap["amount"] = amount
		escrowMap["assetType"] = assetType
		escrowMap["conditionValue"] = conditionValue
		escrowMap["status"] = status
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

var LockFundsInEscrow = transactions.Transaction{
	Tag:         "lockFundsInEscrow",
	Label:       "Lock Funds in Escrow",
	Description: "Lock funds from payer wallet into escrow",
	Method:      "POST",
	Callers: []accesscontrol.Caller{
		{MSP: "Org1MSP", OU: "admin"},
		{MSP: "Org2MSP", OU: "admin"},
	},
	Args: []transactions.Argument{
		{Tag: "escrowId", Label: "Escrow ID", DataType: "string", Required: true},
		{Tag: "payerWalletId", Label: "Payer Wallet ID", DataType: "string", Required: true},
		{Tag: "amount", Label: "Amount to Lock", DataType: "number", Required: true},
		{Tag: "assetId", Label: "Asset ID", DataType: "string", Required: true},
		{Tag: "payerCertHash", Label: "Payer Certificate Hash", DataType: "string", Required: true},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		escrowId, _ := req["escrowId"].(string)
		payerWalletId, _ := req["payerWalletId"].(string)
		amount, _ := req["amount"].(float64)
		assetId, _ := req["assetId"].(string)
		payerCertHash, _ := req["payerCertHash"].(string)

		// 1. Get and verify payer wallet ownership
		payerWalletKey := assets.Key{"@key": "wallet:" + payerWalletId}
		payerWallet, err := payerWalletKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading payer wallet", err.Status())
		}

		// Verify ownership
		if payerWallet.GetProp("ownerCertHash").(string) != payerCertHash {
			return nil, errors.NewCCError("Unauthorized: Certificate hash mismatch", 403)
		}

		// 2. Get wallet balances
		digitalAssetTypes := payerWallet.GetProp("digitalAssetTypes").([]interface{})
		balances := payerWallet.GetProp("balances").([]interface{})

		// Get or initialize escrow balances
		var escrowBalances []interface{}
		if payerWallet.GetProp("escrowBalances") != nil {
			escrowBalances = payerWallet.GetProp("escrowBalances").([]interface{})
		} else {
			// Initialize escrow balances if not present
			escrowBalances = make([]interface{}, len(balances))
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
			case map[string]interface{}:
				refAssetId = strings.Split(ref["@key"].(string), ":")[1]
			case string:
				refAssetId = ref
			}

			if refAssetId == assetId {
				currentBalance := balances[i].(float64)
				fmt.Println(currentBalance)
				fmt.Println(amount)
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
		walletMap := make(map[string]interface{})
		walletMap["@assetType"] = "wallet"
		walletMap["@key"] = "wallet:" + payerWalletId
		walletMap["walletId"] = payerWallet.GetProp("walletId")
		walletMap["ownerId"] = payerWallet.GetProp("ownerId")
		walletMap["ownerCertHash"] = payerWallet.GetProp("ownerCertHash")
		walletMap["balances"] = balances
		walletMap["escrowBalances"] = escrowBalances
		walletMap["digitalAssetTypes"] = digitalAssetTypes
		walletMap["createdAt"] = payerWallet.GetProp("createdAt")

		updatedWallet, err := assets.NewAsset(walletMap)
		if err != nil {
			return nil, errors.WrapError(err, "Failed to update wallet")
		}

		_, err = updatedWallet.Put(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving updated wallet", err.Status())
		}

		// 6. Update escrow status to "Active"
		escrowKey := assets.Key{"@key": "escrow:" + escrowId}
		escrowAsset, err := escrowKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading escrow", err.Status())
		}

		escrowMap := make(map[string]interface{})
		escrowMap["@assetType"] = "escrow"
		escrowMap["@key"] = "escrow:" + escrowId
		escrowMap["escrowId"] = escrowAsset.GetProp("escrowId")
		escrowMap["buyerPubKey"] = escrowAsset.GetProp("buyerPubKey")
		escrowMap["sellerPubKey"] = escrowAsset.GetProp("sellerPubKey")
		escrowMap["amount"] = escrowAsset.GetProp("amount")
		escrowMap["assetType"] = escrowAsset.GetProp("assetType")
		escrowMap["conditionValue"] = escrowAsset.GetProp("conditionValue")
		escrowMap["status"] = "Active" // Update status
		escrowMap["createdAt"] = escrowAsset.GetProp("createdAt")
		escrowMap["buyerCertHash"] = escrowAsset.GetProp("buyerCertHash")

		updatedEscrow, err := assets.NewAsset(escrowMap)
		if err != nil {
			return nil, errors.WrapError(err, "Failed to update escrow")
		}

		_, err = updatedEscrow.Put(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving updated escrow", err.Status())
		}

		// 7. Return response
		response := map[string]interface{}{
			"message":       "Funds locked successfully",
			"escrowId":      escrowId,
			"payerWalletId": payerWalletId,
			"amount":        amount,
			"assetId":       assetId,
			"escrowStatus":  "Active",
		}

		responseJSON, jsonErr := json.Marshal(response)
		if jsonErr != nil {
			return nil, errors.WrapError(nil, "failed to encode response to JSON format")
		}

		return responseJSON, nil
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
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
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
		escrowMap := make(map[string]interface{})
		escrowMap["@assetType"] = "escrow"
		escrowMap["@key"] = "escrow:" + escrowId
		escrowMap["escrowId"] = escrowAsset.GetProp("escrowId")
		escrowMap["buyerPubKey"] = escrowAsset.GetProp("buyerPubKey")
		escrowMap["sellerPubKey"] = escrowAsset.GetProp("sellerPubKey")
		escrowMap["amount"] = escrowAsset.GetProp("amount")
		escrowMap["assetType"] = escrowAsset.GetProp("assetType")
		escrowMap["conditionValue"] = escrowAsset.GetProp("conditionValue")
		escrowMap["status"] = "ReadyForRelease" // Update status
		escrowMap["createdAt"] = escrowAsset.GetProp("createdAt")
		escrowMap["buyerCertHash"] = escrowAsset.GetProp("buyerCertHash")

		updatedEscrow, err := assets.NewAsset(escrowMap)
		if err != nil {
			return nil, errors.WrapError(err, "Failed to update escrow")
		}

		_, err = updatedEscrow.Put(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving updated escrow", err.Status())
		}

		// 6. Return success response
		response := map[string]interface{}{
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
	Tag: "releaseEscrow",
	Args: []transactions.Argument{
		{Tag: "escrowId", DataType: "string", Required: true},
		{Tag: "payerWalletId", Label: "Payer Wallet ID", DataType: "string", Required: true},
		{Tag: "payeeWalletId", Label: "Payee Wallet ID", DataType: "string", Required: true},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		escrowId, _ := req["escrowId"].(string)
		payerWalletId, _ := req["payerWalletId"].(string)
		payeeWalletId, _ := req["payeeWalletId"].(string)

		// 1. Verify escrow status is "ReadyForRelease"
		escrowKey := assets.Key{"@key": "escrow:" + escrowId}
		escrowAsset, err := escrowKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading escrow", err.Status())
		}

		currentStatus := escrowAsset.GetProp("status").(string)
		if currentStatus != "ReadyForRelease" {
			return nil, errors.NewCCError("Escrow is not ready for release", 400)
		}

		escrowAmount := escrowAsset.GetProp("amount").(float64)
		// Get asset reference from escrow
		assetTypeRef := escrowAsset.GetProp("assetType")
		var assetId string
		switch ref := assetTypeRef.(type) {
		case map[string]interface{}:
			assetId = strings.Split(ref["@key"].(string), ":")[1]
		case string:
			assetId = ref
		}

		// 2. Get payer wallet
		payerWalletKey := assets.Key{"@key": "wallet:" + payerWalletId}
		payerWallet, err := payerWalletKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading payer wallet", err.Status())
		}

		// 3. Get payee wallet
		payeeWalletKey := assets.Key{"@key": "wallet:" + payeeWalletId}
		payeeWallet, err := payeeWalletKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading payee wallet", err.Status())
		}

		// 4. Move funds from payer's escrowBalances to payee's balances
		// Update payer wallet
		payerAssetTypes := payerWallet.GetProp("digitalAssetTypes").([]interface{})
		payerBalances := payerWallet.GetProp("balances").([]interface{})
		payerEscrowBalances := payerWallet.GetProp("escrowBalances").([]interface{})

		// Find asset in payer wallet and reduce escrow balance
		payerAssetFound := false
		for i, assetRef := range payerAssetTypes {
			var refAssetId string
			switch ref := assetRef.(type) {
			case map[string]interface{}:
				refAssetId = strings.Split(ref["@key"].(string), ":")[1]
			case string:
				refAssetId = ref
			}

			if refAssetId == assetId {
				currentEscrowBalance := payerEscrowBalances[i].(float64)
				if currentEscrowBalance < escrowAmount {
					return nil, errors.NewCCError("Insufficient escrow balance", 400)
				}
				payerEscrowBalances[i] = currentEscrowBalance - escrowAmount
				payerAssetFound = true
				break
			}
		}

		if !payerAssetFound {
			return nil, errors.NewCCError("Asset not found in payer wallet", 404)
		}

		// Update payee wallet
		payeeAssetTypes := payeeWallet.GetProp("digitalAssetTypes").([]interface{})
		payeeBalances := payeeWallet.GetProp("balances").([]interface{})

		// Initialize payee escrow balances if not present
		var payeeEscrowBalances []interface{}
		if payeeWallet.GetProp("escrowBalances") != nil {
			payeeEscrowBalances = payeeWallet.GetProp("escrowBalances").([]interface{})
		} else {
			payeeEscrowBalances = make([]interface{}, len(payeeBalances))
			for i := range payeeEscrowBalances {
				payeeEscrowBalances[i] = 0.0
			}
		}

		// Find asset in payee wallet and increase balance
		payeeAssetFound := false
		for i, assetRef := range payeeAssetTypes {
			var refAssetId string
			switch ref := assetRef.(type) {
			case map[string]interface{}:
				refAssetId = strings.Split(ref["@key"].(string), ":")[1]
			case string:
				refAssetId = ref
			}

			if refAssetId == assetId {
				currentBalance := payeeBalances[i].(float64)
				payeeBalances[i] = currentBalance + escrowAmount
				payeeAssetFound = true
				break
			}
		}

		// If asset not found in payee wallet, add it
		if !payeeAssetFound {
			payeeAssetTypes = append(payeeAssetTypes, map[string]interface{}{
				"@key": "digitalAsset:" + assetId,
			})
			payeeBalances = append(payeeBalances, escrowAmount)
			payeeEscrowBalances = append(payeeEscrowBalances, 0.0)
		}

		// 5. Save updated payer wallet
		payerWalletMap := make(map[string]interface{})
		payerWalletMap["@assetType"] = "wallet"
		payerWalletMap["@key"] = "wallet:" + payerWalletId
		payerWalletMap["walletId"] = payerWallet.GetProp("walletId")
		payerWalletMap["ownerId"] = payerWallet.GetProp("ownerId")
		payerWalletMap["ownerCertHash"] = payerWallet.GetProp("ownerCertHash")
		payerWalletMap["balances"] = payerBalances
		payerWalletMap["escrowBalances"] = payerEscrowBalances
		payerWalletMap["digitalAssetTypes"] = payerAssetTypes
		payerWalletMap["createdAt"] = payerWallet.GetProp("createdAt")

		updatedPayerWallet, err := assets.NewAsset(payerWalletMap)
		if err != nil {
			return nil, errors.WrapError(err, "Failed to update payer wallet")
		}

		_, err = updatedPayerWallet.Put(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving payer wallet", err.Status())
		}

		// Save updated payee wallet
		payeeWalletMap := make(map[string]interface{})
		payeeWalletMap["@assetType"] = "wallet"
		payeeWalletMap["@key"] = "wallet:" + payeeWalletId
		payeeWalletMap["walletId"] = payeeWallet.GetProp("walletId")
		payeeWalletMap["ownerId"] = payeeWallet.GetProp("ownerId")
		payeeWalletMap["ownerCertHash"] = payeeWallet.GetProp("ownerCertHash")
		payeeWalletMap["balances"] = payeeBalances
		payeeWalletMap["escrowBalances"] = payeeEscrowBalances
		payeeWalletMap["digitalAssetTypes"] = payeeAssetTypes
		payeeWalletMap["createdAt"] = payeeWallet.GetProp("createdAt")

		updatedPayeeWallet, err := assets.NewAsset(payeeWalletMap)
		if err != nil {
			return nil, errors.WrapError(err, "Failed to update payee wallet")
		}

		_, err = updatedPayeeWallet.Put(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving payee wallet", err.Status())
		}

		// Update escrow status to "Released"
		escrowMap := make(map[string]interface{})
		escrowMap["@assetType"] = "escrow"
		escrowMap["@key"] = "escrow:" + escrowId
		escrowMap["escrowId"] = escrowAsset.GetProp("escrowId")
		escrowMap["buyerPubKey"] = escrowAsset.GetProp("buyerPubKey")
		escrowMap["sellerPubKey"] = escrowAsset.GetProp("sellerPubKey")
		escrowMap["amount"] = escrowAsset.GetProp("amount")
		escrowMap["assetType"] = escrowAsset.GetProp("assetType")
		escrowMap["conditionValue"] = escrowAsset.GetProp("conditionValue")
		escrowMap["status"] = "Released"
		escrowMap["createdAt"] = escrowAsset.GetProp("createdAt")
		escrowMap["buyerCertHash"] = escrowAsset.GetProp("buyerCertHash")

		updatedEscrow, err := assets.NewAsset(escrowMap)
		if err != nil {
			return nil, errors.WrapError(err, "Failed to update escrow")
		}

		_, err = updatedEscrow.Put(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving updated escrow", err.Status())
		}

		// Return response
		response := map[string]interface{}{
			"message":       "Escrow funds released successfully",
			"escrowId":      escrowId,
			"payerWalletId": payerWalletId,
			"payeeWalletId": payeeWalletId,
			"amount":        escrowAmount,
			"assetId":       assetId,
			"status":        "Released",
		}

		responseJSON, jsonErr := json.Marshal(response)
		if jsonErr != nil {
			return nil, errors.WrapError(nil, "failed to encode response to JSON format")
		}

		return responseJSON, nil
	},
}

var RefundEscrow = transactions.Transaction{
	Tag: "refundEscrow",
	Args: []transactions.Argument{
		{Tag: "escrowId", DataType: "string", Required: true},
		{Tag: "payerWalletId", Label: "Payer Wallet ID", DataType: "string", Required: true},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		escrowId, _ := req["escrowId"].(string)
		payerWalletId, _ := req["payerWalletId"].(string)

		// 1. Get escrow
		escrowKey := assets.Key{"@key": "escrow:" + escrowId}
		escrowAsset, err := escrowKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading escrow", err.Status())
		}

		// Check escrow is not already released or refunded
		currentStatus := escrowAsset.GetProp("status").(string)
		if currentStatus == "Released" || currentStatus == "Refunded" {
			return nil, errors.NewCCError("Escrow already "+currentStatus, 400)
		}

		escrowAmount := escrowAsset.GetProp("amount").(float64)

		// Get asset reference from escrow
		assetTypeRef := escrowAsset.GetProp("assetType")
		var assetId string
		switch ref := assetTypeRef.(type) {
		case map[string]interface{}:
			assetId = strings.Split(ref["@key"].(string), ":")[1]
		case string:
			assetId = ref
		}

		// 2. Get payer wallet
		payerWalletKey := assets.Key{"@key": "wallet:" + payerWalletId}
		payerWallet, err := payerWalletKey.Get(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error reading payer wallet", err.Status())
		}

		// 3. Move funds from payer's escrowBalances back to balances
		payerAssetTypes := payerWallet.GetProp("digitalAssetTypes").([]interface{})
		payerBalances := payerWallet.GetProp("balances").([]interface{})
		payerEscrowBalances := payerWallet.GetProp("escrowBalances").([]interface{})

		// Find asset in payer wallet
		payerAssetFound := false
		for i, assetRef := range payerAssetTypes {
			var refAssetId string
			switch ref := assetRef.(type) {
			case map[string]interface{}:
				refAssetId = strings.Split(ref["@key"].(string), ":")[1]
			case string:
				refAssetId = ref
			}

			if refAssetId == assetId {
				currentBalance := payerBalances[i].(float64)
				currentEscrowBalance := payerEscrowBalances[i].(float64)

				if currentEscrowBalance < escrowAmount {
					return nil, errors.NewCCError("Insufficient escrow balance", 400)
				}

				// Move funds from escrow back to available balance
				payerBalances[i] = currentBalance + escrowAmount
				payerEscrowBalances[i] = currentEscrowBalance - escrowAmount
				payerAssetFound = true
				break
			}
		}

		if !payerAssetFound {
			return nil, errors.NewCCError("Asset not found in payer wallet", 404)
		}

		// 4. Save updated payer wallet
		payerWalletMap := make(map[string]interface{})
		payerWalletMap["@assetType"] = "wallet"
		payerWalletMap["@key"] = "wallet:" + payerWalletId
		payerWalletMap["walletId"] = payerWallet.GetProp("walletId")
		payerWalletMap["ownerId"] = payerWallet.GetProp("ownerId")
		payerWalletMap["ownerCertHash"] = payerWallet.GetProp("ownerCertHash")
		payerWalletMap["balances"] = payerBalances
		payerWalletMap["escrowBalances"] = payerEscrowBalances
		payerWalletMap["digitalAssetTypes"] = payerAssetTypes
		payerWalletMap["createdAt"] = payerWallet.GetProp("createdAt")

		updatedPayerWallet, err := assets.NewAsset(payerWalletMap)
		if err != nil {
			return nil, errors.WrapError(err, "Failed to update payer wallet")
		}

		_, err = updatedPayerWallet.Put(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving payer wallet", err.Status())
		}

		// 5. Update escrow status to "Refunded"
		escrowMap := make(map[string]interface{})
		escrowMap["@assetType"] = "escrow"
		escrowMap["@key"] = "escrow:" + escrowId
		escrowMap["escrowId"] = escrowAsset.GetProp("escrowId")
		escrowMap["buyerPubKey"] = escrowAsset.GetProp("buyerPubKey")
		escrowMap["sellerPubKey"] = escrowAsset.GetProp("sellerPubKey")
		escrowMap["amount"] = escrowAsset.GetProp("amount")
		escrowMap["assetType"] = escrowAsset.GetProp("assetType")
		escrowMap["conditionValue"] = escrowAsset.GetProp("conditionValue")
		escrowMap["status"] = "Refunded"
		escrowMap["createdAt"] = escrowAsset.GetProp("createdAt")
		escrowMap["buyerCertHash"] = escrowAsset.GetProp("buyerCertHash")

		updatedEscrow, err := assets.NewAsset(escrowMap)
		if err != nil {
			return nil, errors.WrapError(err, "Failed to update escrow")
		}

		_, err = updatedEscrow.Put(stub)
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "Error saving updated escrow", err.Status())
		}

		// Return response
		response := map[string]interface{}{
			"message":       "Escrow funds refunded successfully",
			"escrowId":      escrowId,
			"payerWalletId": payerWalletId,
			"amount":        escrowAmount,
			"assetId":       assetId,
			"status":        "Refunded",
		}

		responseJSON, jsonErr := json.Marshal(response)
		if jsonErr != nil {
			return nil, errors.WrapError(nil, "failed to encode response to JSON format")
		}

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
		// PPS: Cross-check method is needed...
		{
			Tag:         "uuid",
			Label:       "UUID",
			Description: "UUID of the Digital Asset to read",
			DataType:    "string",
			Required:    true,
		},
	},

	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
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
