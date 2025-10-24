package application

import (
	"encoding/binary"
	"fmt"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/0xAtelerix/sdk/gosdk/external"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
)

// NOTE: This implementation is specific to the Solana programs in gosdk/solana-programs deployed on Devnet
// If you're using your own Solana program, you'll need to modify:
// 1. The account structure (number and order of accounts)
// 2. The data payload format
// 3. PDA derivation logic if using different seeds
// 4. Program IDs and other constants

const (
	AppchainProgIDStr = "33ZWF8uKZd4GgsXKMmHNnCMu82MW2yGe1gkTE27YC8Ke" // Deployed on solana devnet
	MintPubkeyStr     = "sN8167EqsyWten98aTVd4N138GjnZZDEGjX6MhZQfC8"
	TokenProgID       = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA" //nolint:gosec // This is Solana's SPL Token Program ID, not a credential
	XUserPubkeyStr    = "E3Ct6ynJFr8y3bYWcQ8NeiugWLPFqPvRbfXRQvcqZn4M"
)

// deriveATA derives the Associated Token Account address for a given mint and owner
func deriveATA(mint, owner string) string {
	mintPubkey := common.PublicKeyFromString(mint)
	ownerPubkey := common.PublicKeyFromString(owner)
	tokenProgramID := common.PublicKeyFromString(TokenProgID)

	// Find PDA for ATA: find_program_address([owner, TOKEN_PROGRAM_ID, mint], ASSOCIATED_TOKEN_PROGRAM_ID)
	// Using the standard Solana Associated Token Program ID
	associatedTokenProgramID := common.SPLAssociatedTokenAccountProgramID

	seeds := [][]byte{
		ownerPubkey.Bytes(),
		tokenProgramID.Bytes(),
		mintPubkey.Bytes(),
	}

	ata, _, err := common.FindProgramAddress(seeds, associatedTokenProgramID)
	if err != nil {
		// Fallback to placeholder if derivation fails
		return "DerivedATAPubkeyHere"
	}

	return ata.ToBase58()
}

// deriveMintAuthorityPDA derives the PDA for mint authority using the appchain program ID
func deriveMintAuthorityPDA(programID string) (string, uint8) {
	progPubkey := common.PublicKeyFromString(programID)
	seeds := [][]byte{[]byte("mint_authority")}

	pda, bump, err := common.FindProgramAddress(seeds, progPubkey)
	if err != nil {
		// Should never happen with valid inputs
		return "", 0
	}

	return pda.ToBase58(), bump
}

// createSolanaMintPayload creates a cross-chain transaction payload for minting tokens on Solana.
// This implementation is specific to the SDK's Solana program (gosdk/solana-programs) which:
// - Expects exactly 5 accounts in this order:
//  1. Appchain Program (the program itself)
//  2. Mint Account (the token mint)
//  3. Associated Token Account (recipient's ATA)
//  4. Mint Authority PDA
//  5. Token Program
//
// - Requires payload data in format: recipient pubkey (32 bytes) + amount (8 bytes, little-endian)
//
// If you're using a custom Solana program, you'll need to modify this function to match your program's:
// - Account structure and ordering
// - Instruction data format
// - PDA derivation logic (if using different seeds)
// - Access control and signing requirements
func createSolanaMintPayload(amount uint64) (apptypes.ExternalTransaction, error) {
	// Build Solana mint payload
	appchainProg := types.AccountMeta{
		PubKey:     common.PublicKeyFromString(AppchainProgIDStr),
		IsSigner:   false,
		IsWritable: false,
	}
	mintAcc := types.AccountMeta{
		PubKey:     common.PublicKeyFromString(MintPubkeyStr),
		IsSigner:   false,
		IsWritable: true, // Mint authority writable
	}
	ataAcc := types.AccountMeta{
		PubKey:     common.PublicKeyFromString(deriveATA(MintPubkeyStr, XUserPubkeyStr)),
		IsSigner:   false,
		IsWritable: true, // Recipient ATA
	}

	// Add mint authority PDA (4th specific account)
	authorityPDA, _ := deriveMintAuthorityPDA(AppchainProgIDStr)
	authorityAcc := types.AccountMeta{
		PubKey:     common.PublicKeyFromString(authorityPDA),
		IsSigner:   false, // Signed via seeds in appchain
		IsWritable: false,
	}

	tokenProg := types.AccountMeta{
		PubKey:     common.PublicKeyFromString(TokenProgID),
		IsSigner:   false,
		IsWritable: false,
	}

	// Appchain-specific data: recipient (32 bytes) + amount (8 bytes)
	// Total: 40 bytes (matches Rust program format)
	appchainData := make([]byte, 0, 32+8)

	// 1. Add recipient (32 bytes)
	recipientPubkey := common.PublicKeyFromString(XUserPubkeyStr)
	appchainData = append(appchainData, recipientPubkey.Bytes()...)

	// 2. Add amount (8 bytes, little-endian)
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, amount)
	appchainData = append(appchainData, amountBytes...)

	// Build account list: appchainProg first, then specifics (mint, ata, authority, token)
	// Total: 5 accounts will be passed to appchain after CPI
	// In appchain: [0]=program, [1]=mint, [2]=ata, [3]=authority, [4]=token
	exTx, err := external.NewExTxBuilder(appchainData, gosdk.SolanaDevnetChainID).
		AddSolanaAccounts([]types.AccountMeta{appchainProg, mintAcc, ataAcc, authorityAcc, tokenProg}).
		Build()
	if err != nil {
		return apptypes.ExternalTransaction{}, fmt.Errorf("build payload: %w", err)
	}

	return exTx, nil
}
