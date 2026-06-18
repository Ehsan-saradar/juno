package starknet_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/NethermindEth/juno/starknet"
	"github.com/stretchr/testify/require"
)

// loadFeederTestdata returns the raw bytes of a feeder testdata fixture
// relative to the clients/feeder/testdata directory.
func loadFeederTestdata(t *testing.T, relPath string) []byte {
	t.Helper()
	full := filepath.Join("..", "clients", "feeder", "testdata", relPath)
	data, err := os.ReadFile(full)
	require.NoError(t, err, "load %s", full)
	return data
}

func TestPreConfirmedUpdateEnvelope_UnmarshalJSON(t *testing.T) {
	// The latest tag carries block_number on the wire (the caller is
	// discovering the tip), so the envelope unwraps it into BlockNumber.
	t.Run("latest tag response", func(t *testing.T) {
		t.Run("Full decodes as a new round carrying block_number", func(t *testing.T) {
			raw := loadFeederTestdata(t, "sepolia/preconfirmed/latest/full.json")

			var env starknet.PreConfirmedUpdateEnvelope
			require.NoError(t, json.Unmarshal(raw, &env))
			require.Equal(t, uint64(10936237), env.BlockNumber, "latest endpoint carries block_number")

			full, ok := env.Update.(starknet.PreConfirmedBlock)
			require.True(t, ok, "expected PreConfirmedBlock, got %T", env.Update)
			require.Equal(t, "0x1cbe25d9", full.BlockIdentifier)
			require.Equal(t, "PRE_CONFIRMED", full.Status)
			require.NotZero(t, full.Timestamp)
			require.NotNil(t, full.SequencerAddress)
			require.NotNil(t, full.L1GasPrice)
		})

		t.Run("Delta decodes carrying block_number", func(t *testing.T) {
			// Appended-delta poll: changed=true, no timestamp, carrying the
			// transactions/receipts/state-diffs appended since the known tx count.
			raw := loadFeederTestdata(t, "sepolia/preconfirmed/latest/0x1cbe25d9/3.json")

			var env starknet.PreConfirmedUpdateEnvelope
			require.NoError(t, json.Unmarshal(raw, &env))
			require.Equal(t, uint64(10936237), env.BlockNumber, "latest endpoint carries block_number")

			delta, ok := env.Update.(starknet.PreConfirmedDeltaUpdate)
			require.True(t, ok, "expected PreConfirmedDeltaUpdate, got %T", env.Update)
			require.Equal(t, "0x1cbe25d9", delta.BlockIdentifier)
			require.Len(t, delta.Transactions, 1)
			require.Len(t, delta.Receipts, 1)
			require.Len(t, delta.TransactionStateDiffs, 1)
			// The appended tx/receipt decoded into real, non-nil wire values.
			require.NotNil(t, delta.Transactions[0].Hash)
			require.NotNil(t, delta.Receipts[0].TransactionHash)
		})

		t.Run("NoChange decodes when nothing was appended", func(t *testing.T) {
			raw := loadFeederTestdata(t, "sepolia/preconfirmed/latest/0x1cbe25d9/4.json")

			var env starknet.PreConfirmedUpdateEnvelope
			require.NoError(t, json.Unmarshal(raw, &env))

			_, ok := env.Update.(starknet.PreConfirmedNoChange)
			require.True(t, ok, "expected PreConfirmedNoChange, got %T", env.Update)
		})
	})

	// The explicit-number endpoint omits block_number (the caller already knows
	// the height), so the envelope leaves BlockNumber at zero.
	t.Run("explicit number response", func(t *testing.T) {
		t.Run("Full decodes as a new round omitting block_number", func(t *testing.T) {
			raw := loadFeederTestdata(t, "sepolia/preconfirmed/1781802365/full.json")

			var env starknet.PreConfirmedUpdateEnvelope
			require.NoError(t, json.Unmarshal(raw, &env))
			require.Zero(t, env.BlockNumber, "numbered endpoint omits block_number")

			full, ok := env.Update.(starknet.PreConfirmedBlock)
			require.True(t, ok, "expected PreConfirmedBlock, got %T", env.Update)
			require.Equal(t, "0x1857317c", full.BlockIdentifier)
			require.Equal(t, "PRE_CONFIRMED", full.Status)
			require.NotZero(t, full.Timestamp)
		})

		t.Run("Delta decodes omitting block_number", func(t *testing.T) {
			raw := loadFeederTestdata(t, "sepolia/preconfirmed/1781802365/0x1857317c/2.json")

			var env starknet.PreConfirmedUpdateEnvelope
			require.NoError(t, json.Unmarshal(raw, &env))
			require.Zero(t, env.BlockNumber, "numbered endpoint omits block_number")

			delta, ok := env.Update.(starknet.PreConfirmedDeltaUpdate)
			require.True(t, ok, "expected PreConfirmedDeltaUpdate, got %T", env.Update)
			require.Equal(t, "0x1857317c", delta.BlockIdentifier)
		})

		t.Run("NoChange decodes when nothing was appended", func(t *testing.T) {
			raw := loadFeederTestdata(t, "sepolia/preconfirmed/1781802365/0x1857317c/4.json")

			var env starknet.PreConfirmedUpdateEnvelope
			require.NoError(t, json.Unmarshal(raw, &env))

			_, ok := env.Update.(starknet.PreConfirmedNoChange)
			require.True(t, ok, "expected PreConfirmedNoChange, got %T", env.Update)
		})
	})

	// Variant discrimination and malformed payloads are endpoint-independent.
	t.Run("discrimination and malformed payloads", func(t *testing.T) {
		t.Run("changed absent returns error", func(t *testing.T) {
			var env starknet.PreConfirmedUpdateEnvelope
			require.Error(t, json.Unmarshal([]byte(`{}`), &env))
		})

		t.Run("changed=false ignores additional fields", func(t *testing.T) {
			// A NoChange response is identified purely by changed=false; any
			// additional fields must be ignored.
			raw := []byte(`{
				"changed": false,
				"block_identifier": "ignored",
				"transactions": [{"transaction_hash": "0x1"}]
			}`)

			var env starknet.PreConfirmedUpdateEnvelope
			require.NoError(t, json.Unmarshal(raw, &env))

			_, ok := env.Update.(starknet.PreConfirmedNoChange)
			require.True(t, ok, "expected PreConfirmedNoChange, got %T", env.Update)
		})

		t.Run("invalid JSON returns error", func(t *testing.T) {
			var env starknet.PreConfirmedUpdateEnvelope
			require.Error(t, json.Unmarshal([]byte(`{`), &env))
		})

		t.Run("invalid Full payload returns error", func(t *testing.T) {
			// `timestamp` must be a uint64; passing an object surfaces the inner
			// decode error rather than being silently dropped.
			raw := []byte(`{"changed": true, "timestamp": {}}`)
			var env starknet.PreConfirmedUpdateEnvelope
			require.Error(t, json.Unmarshal(raw, &env))
		})

		t.Run("invalid Delta payload returns error", func(t *testing.T) {
			raw := []byte(`{"changed": true, "block_identifier": 7}`)
			var env starknet.PreConfirmedUpdateEnvelope
			require.Error(t, json.Unmarshal(raw, &env))
		})
	})
}
