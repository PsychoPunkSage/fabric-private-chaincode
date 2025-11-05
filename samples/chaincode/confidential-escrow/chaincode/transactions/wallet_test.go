package transactions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/hyperledger-labs/cc-tools/assets"
	asset "github.com/hyperledger/fabric-private-chaincode/samples/chaincode/confidential-escrow/chaincode/assets"
	"github.com/hyperledger/fabric-private-chaincode/samples/chaincode/confidential-escrow/chaincode/testutils"
)

// TestMain runs before all tests to initialize cc-tools
func TestMain(m *testing.M) {
	// Initialize asset types
	assetTypeList := []assets.AssetType{
		asset.Wallet,
		asset.DigitalAssetToken,
		asset.UserDirectory,
		asset.Escrow,
	}
	assets.InitAssetList(assetTypeList)

	// Run tests
	m.Run()
}

func TestCreateWallet_Success(t *testing.T) {
	// Create a fresh mock blockchain stub for this test
	wrapper, mockStub := testutils.NewMockStubWrapper()

	// Define the input arguments for creating a wallet
	args := map[string]any{
		"walletId":      "alice-savings",          // User-friendly nickname
		"ownerPubKey":   "alice-public-key-12345", // Alice's public key
		"ownerCertHash": "alice-cert-hash-xyz",    // Alice's certificate hash
	}

	// Execute the CreateWallet transaction
	response, err := CreateWallet.Routine(wrapper.StubWrapper, args)
	testutils.AssertNoError(t, err, "wallet creation should succeed")

	// Step 2: Parse the response to check wallet properties
	var createdWallet map[string]any
	if err := json.Unmarshal(response, &createdWallet); err != nil {
		t.Fatalf("Failed to parse wallet response: %v", err)
	}

	// Step 3: Verify the nickname was stored correctly
	testutils.AssertEqual(t, "alice-savings", createdWallet["walletId"], "walletId mismatch")

	// Step 4: Verify the public key was stored correctly
	testutils.AssertEqual(t, "alice-public-key-12345", createdWallet["ownerPubKey"], "owner Pub key mismatch")

	// Step 5: Verify the certificate hash was stored correctly
	testutils.AssertEqual(t, "alice-cert-hash-xyz", createdWallet["ownerCertHash"], "cert hash mismatch")

	// Step 6: Verify that a UUID was generated (in the @key field)
	walletKey, exists := createdWallet["@key"].(string)
	if !exists {
		t.Fatal("Expected wallet to have a @key field with UUID")
	}

	// The key should be in format "wallet:<UUID>"
	if len(walletKey) < 8 || walletKey[:7] != "wallet:" {
		t.Errorf("Expected key format 'wallet:<UUID>', got: %s", walletKey)
	}

	// Extract the UUID portion (everything after "wallet:")
	walletUUID := walletKey[7:]

	// Step 7: Verify the wallet was actually saved to the mock ledger
	_, exists = mockStub.State[walletKey]
	if !exists {
		t.Errorf("Expected wallet to be saved with key '%s'", walletKey)
	}

	// Step 8: Verify UserDirectory was created
	// The UserDirectory should map the hash of the public key to the wallet UUID

	// Compute the hash of Alice's public key
	hash := sha256.Sum256([]byte("alice-public-key-12345"))
	pubKeyHash := hex.EncodeToString(hash[:])

	// Build the expected UserDirectory key
	userDirKey := "userdir:" + pubKeyHash

	// Check if UserDirectory entry exists
	userDirBytes, exists := mockStub.State[userDirKey]
	if !exists {
		t.Fatalf("Expected UserDirectory entry to exist at key '%s'", userDirKey)
	}

	// Step 9: Verify UserDirectory contains the correct wallet UUID
	var userDir map[string]any
	if err := json.Unmarshal(userDirBytes, &userDir); err != nil {
		t.Fatalf("Failed to parse UserDirectory: %v", err)
	}

	testutils.AssertEqual(t, walletUUID, userDir["walletUUID"], "UserDir has different walletUUID")

	// Step 10: Verify empty balance arrays were initialized
	balances, ok := createdWallet["balances"].([]any)
	if !ok || len(balances) != 0 {
		t.Errorf("Expected empty balances array, got: %v", createdWallet["balances"])
	}

	escrowBalances, ok := createdWallet["escrowBalances"].([]any)
	if !ok || len(escrowBalances) != 0 {
		t.Errorf("Expected empty escrowBalances array, got: %v", createdWallet["escrowBalances"])
	}

	t.Log("✓ Wallet created successfully with all expected properties")
}
