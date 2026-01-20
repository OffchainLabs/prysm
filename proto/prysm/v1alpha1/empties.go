package eth

// EmptyBuilderPendingPayment is a shared zero-value payment used to clear entries.
var EmptyBuilderPendingPayment = &BuilderPendingPayment{
	Withdrawal: &BuilderPendingWithdrawal{
		FeeRecipient: make([]byte, 20),
	},
}
