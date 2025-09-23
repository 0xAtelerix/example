package application

import "github.com/ledgerwatch/erigon-lib/kv"

const (
	AccountsBucket = "appaccounts" // token+account -> value
)

func Tables() kv.TableCfg {
	return kv.TableCfg{
		AccountsBucket: {},
	}
}
