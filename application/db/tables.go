package db

import (
	"github.com/ledgerwatch/erigon-lib/kv"
)

//nolint:gochecknoglobals //only for sharding testing
var accs = make(map[string]map[string][]byte)

const (
	AccountsBucket = "appaccounts" // token+account -> value
)

func Tables() kv.TableCfg {
	return kv.TableCfg{
		AccountsBucket: {},
	}
}

func AccountKey(sender string, token string) []byte {
	return []byte(token + sender)
}

func GetAccountKey(sender string, token string) []byte {
	acc, ok := accs[sender]
	if !ok {
		acc = make(map[string][]byte)
		accs[sender] = acc
	}

	t, ok := acc[token]
	if !ok {
		t = AccountKey(sender, token)
		acc[token] = t
	}

	return t
}
