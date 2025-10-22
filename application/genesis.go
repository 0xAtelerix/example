package application

import (
	"context"
	"fmt"

	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog/log"
)

// GenesisAccount represents an initial account balance configured for genesis.
type GenesisAccount struct {
	Address string
	Token   string
	Balance uint64
}

// GetDefaultGenesisAccounts returns the default genesis accounts with initial balances
func GetDefaultGenesisAccounts() []GenesisAccount {
	return []GenesisAccount{
		// Alice - Well funded
		{Address: "alice", Token: "USDT", Balance: 1000000000000}, // 1,000,000 USDT (6 decimals)
		{Address: "alice", Token: "BTC", Balance: 1000000000},     // 10 BTC (8 decimals)
		{Address: "alice", Token: "ETH", Balance: 10000000000},    // 100 ETH (8 decimals)

		// Bob - Well funded
		{Address: "bob", Token: "USDT", Balance: 1000000000000}, // 1,000,000 USDT
		{Address: "bob", Token: "BTC", Balance: 1000000000},     // 10 BTC
		{Address: "bob", Token: "ETH", Balance: 10000000000},    // 100 ETH

		// Charlie - Medium funded
		{Address: "charlie", Token: "USDT", Balance: 500000000000}, // 500,000 USDT
		{Address: "charlie", Token: "BTC", Balance: 500000000},     // 5 BTC
		{Address: "charlie", Token: "ETH", Balance: 5000000000},    // 50 ETH

		// Dave - Medium funded
		{Address: "dave", Token: "USDT", Balance: 500000000000}, // 500,000 USDT
		{Address: "dave", Token: "BTC", Balance: 500000000},     // 5 BTC
		{Address: "dave", Token: "ETH", Balance: 5000000000},    // 50 ETH

		// Eve - Small funded
		{Address: "eve", Token: "USDT", Balance: 100000000000}, // 100,000 USDT
		{Address: "eve", Token: "BTC", Balance: 100000000},     // 1 BTC
		{Address: "eve", Token: "ETH", Balance: 1000000000},    // 10 ETH
	}
}

// InitializeGenesis sets up initial account balances
func InitializeGenesis(ctx context.Context, db kv.RwDB) error {
	if db == nil {
		return ErrDatabaseNil
	}

	// Begin transaction
	tx, err := db.BeginRw(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	// Check if genesis has already been initialized by looking for genesis marker
	genesisMarker := []byte("genesis_initialized")

	existing, err := tx.GetOne(AccountsBucket, genesisMarker)
	if err != nil {
		return fmt.Errorf("failed to check genesis marker: %w", err)
	}

	if len(existing) > 0 {
		log.Info().Msg("Genesis already initialized, skipping...")

		return tx.Commit()
	}

	log.Info().Msg("First startup detected - initializing genesis state...")

	// Initialize accounts with genesis balances
	for _, account := range GetDefaultGenesisAccounts() {
		// Set account balance
		accountKey := AccountKey(account.Address, account.Token)
		balance := uint256.NewInt(account.Balance)

		err = tx.Put(AccountsBucket, accountKey, balance.Bytes())
		if err != nil {
			return fmt.Errorf("failed to set genesis balance for %s:%s: %w",
				account.Address, account.Token, err)
		}

		log.Info().
			Str("address", account.Address).
			Str("token", account.Token).
			Uint64("balance", account.Balance).
			Msg("Initialized account balance")
	}

	// Set genesis marker to indicate initialization is complete
	err = tx.Put(AccountsBucket, genesisMarker, []byte("true"))
	if err != nil {
		return fmt.Errorf("failed to set genesis marker: %w", err)
	}

	log.Info().Msg("Genesis initialization completed successfully!")

	return tx.Commit()
}
