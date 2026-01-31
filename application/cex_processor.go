package application

import (
	"context"

	"github.com/0xAtelerix/sdk/gosdk"
	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/rs/zerolog/log"
)

// CEXProcessor implements gosdk.CEXStreamProcessor for the application layer.
// It reads full order book data on demand via the CEXDataAccessor.
type CEXProcessor struct {
	accessor gosdk.CEXDataAccessor
}

// NewCEXProcessor creates a new CEX stream processor.
// Returns nil if accessor is nil (CEX data not available).
func NewCEXProcessor(accessor gosdk.CEXDataAccessor) *CEXProcessor {
	if accessor == nil {
		return nil
	}

	return &CEXProcessor{accessor: accessor}
}

// ProcessCEXStream reads full order book data for each ref and processes it.
//
//nolint:unparam // v1 stub: will return real data when KV storage is added
func (p *CEXProcessor) ProcessCEXStream(
	ctx context.Context,
	refs []apptypes.CEXOrderBookRef,
	_ kv.RwTx,
) ([]apptypes.ExternalTransaction, error) {
	for _, ref := range refs {
		ob, err := p.accessor.ReadCEXOrderBook(ctx, ref.Exchange, ref.Symbol, ref.FetchedAt)
		if err != nil {
			log.Error().Err(err).
				Str("exchange", ref.Exchange).
				Str("symbol", ref.Symbol).
				Int64("fetchedAt", ref.FetchedAt).
				Msg("failed to read CEX order book")

			continue
		}

		logEvt := log.Info().
			Str("exchange", ob.Exchange).
			Str("symbol", ob.Symbol).
			Int("bids", len(ob.Bids)).
			Int("asks", len(ob.Asks)).
			Int64("fetchedAt", ob.FetchedAt)

		if len(ob.Bids) > 0 {
			logEvt = logEvt.Str("topBid", ob.Bids[0].Price).Str("topBidQty", ob.Bids[0].Quantity)
		}

		if len(ob.Asks) > 0 {
			logEvt = logEvt.Str("topAsk", ob.Asks[0].Price).Str("topAskQty", ob.Asks[0].Quantity)
		}

		logEvt.Msg("processing CEX order book")
	}

	return nil, nil
}
