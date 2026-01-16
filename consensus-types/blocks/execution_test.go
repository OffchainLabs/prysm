func TestIsEmptyExecutionData(t *testing.T) {
	t.Run("nil data", func(t *testing.T) {
		empty, err := blocks.IsEmptyExecutionData(nil)
		require.NoError(t, err)
		assert.Equal(t, true, empty)
	})

	t.Run("empty payload", func(t *testing.T) {
		payload, err := blocks.WrappedExecutionPayload(&enginev1.ExecutionPayload{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
		})
		require.NoError(t, err)

		empty, err := blocks.IsEmptyExecutionData(payload)
		require.NoError(t, err)
		assert.Equal(t, true, empty)
	})

	t.Run("non-empty payload - has block number", func(t *testing.T) {
		payload, err := blocks.WrappedExecutionPayload(&enginev1.ExecutionPayload{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			BlockNumber:   123,
		})
		require.NoError(t, err)

		empty, err := blocks.IsEmptyExecutionData(payload)
		require.NoError(t, err)
		assert.Equal(t, false, empty)
	})

	t.Run("non-empty payload - has parent hash", func(t *testing.T) {
		parentHash := make([]byte, fieldparams.RootLength)
		parentHash[0] = 1 // non-zero byte

		payload, err := blocks.WrappedExecutionPayload(&enginev1.ExecutionPayload{
			ParentHash:    parentHash,
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
		})
		require.NoError(t, err)

		empty, err := blocks.IsEmptyExecutionData(payload)
		require.NoError(t, err)
		assert.Equal(t, false, empty)
	})

	t.Run("non-empty payload - has transactions", func(t *testing.T) {
		payload, err := blocks.WrappedExecutionPayload(&enginev1.ExecutionPayload{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  [][]byte{{1, 2, 3}},
		})
		require.NoError(t, err)

		empty, err := blocks.IsEmptyExecutionData(payload)
		require.NoError(t, err)
		assert.Equal(t, false, empty)
	})
}
