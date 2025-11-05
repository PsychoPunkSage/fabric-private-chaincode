package testutils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// TestFixtures provides common test data for unit tests
type TestFixtures struct {
	// User credentials
	BuyerPubKey     string
	BuyerCertHash   string
	BuyerPubKeyHash string
	BuyerWalletID   string
	BuyerWalletUUID string

	SellerPubKey     string
	SellerCertHash   string
	SellerPubKeyHash string
	SellerWalletID   string
	SellerWalletUUID string

	IssuerCertHash string

	// Asset data
	AssetID     string
	AssetSymbol string
	AssetName   string

	// Escrow data
	EscrowID string
	ParcelID string
	Secret   string
	Amount   float64
}

// NewTestFixtures creates a standard set of test data
func NewTestFixtures() *TestFixtures {
	buyerPubKey := "buyer-public-key-123"
	sellerPubKey := "seller-public-key-456"

	buyerHash := sha256.Sum256([]byte(buyerPubKey))
	buyerPubKeyHash := hex.EncodeToString(buyerHash[:])

	sellerHash := sha256.Sum256([]byte(sellerPubKey))
	sellerPubKeyHash := hex.EncodeToString(sellerHash[:])

	return &TestFixtures{
		BuyerPubKey:      buyerPubKey,
		BuyerCertHash:    "buyer-cert-hash",
		BuyerPubKeyHash:  buyerPubKeyHash,
		BuyerWalletID:    "buyer-wallet-id",
		BuyerWalletUUID:  "buyer-wallet-uuid",
		SellerPubKey:     sellerPubKey,
		SellerCertHash:   "seller-cert-hash",
		SellerPubKeyHash: sellerPubKeyHash,
		SellerWalletID:   "seller-wallet-id",
		SellerWalletUUID: "seller-wallet-uuid",
		IssuerCertHash:   "issuer-cert-hash",
		AssetID:          "test-asset-id",
		AssetSymbol:      "TST",
		AssetName:        "Test Token",
		EscrowID:         "test-escrow-id",
		ParcelID:         "parcel-123",
		Secret:           "secret-key",
		Amount:           100.0,
	}
}

// CreateMockWallet creates a wallet asset in the mock state
// walletID is the user-provided nickname
// walletUUID is the cc-tools generated unique identifier
func (f *TestFixtures) CreateMockWallet(mockStub *MockStub, pubKey, certHash, walletID, walletUUID string, assetID string, balance, escrowBalance float64) error {
	walletMap := map[string]interface{}{
		"@assetType":     "wallet",
		"@key":           "wallet:" + walletUUID, // CC-tools composite key
		"walletId":       walletID,               // User-provided nickname
		"ownerPubKey":    pubKey,
		"ownerCertHash":  certHash,
		"balances":       []interface{}{balance},
		"escrowBalances": []interface{}{escrowBalance},
		"digitalAssetTypes": []interface{}{
			map[string]interface{}{
				"@key": "digitalAsset:" + assetID,
			},
		},
		"createdAt": time.Now(),
	}

	walletJSON, err := json.Marshal(walletMap)
	if err != nil {
		return err
	}

	// Store by UUID (the actual ledger key)
	return mockStub.PutState("wallet:"+walletUUID, walletJSON)
}

// CreateMockUserDir creates a user directory entry in the mock state
// The UserDirectory maps publicKeyHash -> walletUUID (NOT walletID)
func (f *TestFixtures) CreateMockUserDir(mockStub *MockStub, pubKeyHash, walletUUID, certHash string) error {
	userDirMap := map[string]interface{}{
		"@assetType":    "userdir",
		"@key":          "userdir:" + pubKeyHash,
		"publicKeyHash": pubKeyHash,
		"walletUUID":    walletUUID, // References the UUID, not the ID
		"certHash":      certHash,
	}

	userDirJSON, err := json.Marshal(userDirMap)
	if err != nil {
		return err
	}

	return mockStub.PutState("userdir:"+pubKeyHash, userDirJSON)
}

// CreateMockDigitalAsset creates a digital asset in the mock state
// assetID is cc-tools generated UUID
// symbol is user-provided unique identifier
func (f *TestFixtures) CreateMockDigitalAsset(mockStub *MockStub, assetID, symbol, name, issuerHash string, totalSupply float64) error {
	assetMap := map[string]interface{}{
		"@assetType":  "digitalAsset",
		"@key":        "digitalAsset:" + assetID, // CC-tools composite key uses UUID
		"name":        name,
		"symbol":      symbol, // This is the unique key property (IsKey: true)
		"decimals":    2.0,
		"totalSupply": totalSupply,
		"issuerHash":  issuerHash,
		"owner":       "test-owner",
		"issuedAt":    time.Now(),
	}

	assetJSON, err := json.Marshal(assetMap)
	if err != nil {
		return err
	}

	return mockStub.PutState("digitalAsset:"+assetID, assetJSON)
}

// CreateMockEscrow creates an escrow contract in the mock state
// escrowID is user-provided unique identifier (IsKey: true)
func (f *TestFixtures) CreateMockEscrow(mockStub *MockStub, escrowID, buyerWalletUUID, sellerWalletUUID, assetID, parcelID, conditionValue, status, buyerCertHash string, amount float64) error {
	escrowMap := map[string]interface{}{
		"@assetType":       "escrow",
		"@key":             "escrow:" + escrowID, // Uses escrowID as the key
		"escrowId":         escrowID,             // User-provided (IsKey: true)
		"buyerPubKey":      f.BuyerPubKey,
		"sellerPubKey":     f.SellerPubKey,
		"buyerWalletUUID":  buyerWalletUUID,  // References wallet UUID
		"sellerWalletUUID": sellerWalletUUID, // References wallet UUID
		"amount":           amount,
		"assetType": map[string]interface{}{
			"@key": "digitalAsset:" + assetID, // References asset UUID
		},
		"parcelId":       parcelID,
		"conditionValue": conditionValue,
		"status":         status,
		"createdAt":      time.Now(),
		"buyerCertHash":  buyerCertHash,
	}

	escrowJSON, err := json.Marshal(escrowMap)
	if err != nil {
		return err
	}

	return mockStub.PutState("escrow:"+escrowID, escrowJSON)
}

// ComputeConditionHash computes the escrow condition hash
func (f *TestFixtures) ComputeConditionHash(secret, parcelID string) string {
	hash := sha256.Sum256([]byte(secret + parcelID))
	return hex.EncodeToString(hash[:])
}
