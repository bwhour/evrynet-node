package test

//These contants required test environment. The params bellow are the address on testNode
const (
	// this is just a random address to run negative test
	ownerPK                        = "ce900e4057ef7253ce737dccf3979ec4e74a19d595e8cc30c6c5ea92dfdd37f1"
	ownerAddrStr                   = "EQzeFSroGjB4xodbMYP1qydXeWYgypGSJe"
	normalAddress                  = "EUgzD7BJaiB1AtYTeR2DfhYwf4dPb4jj5E"
	senderPK                       = "0BC15BA68AAEC11F2638BC7C98BBA3E17A1D1F4BD5C27BB6043BA68D7F262962"
	senderAddrStr                  = "ERxUj1gQj8KSoPnhqZUfXwj62Wzyv3rije"
	contractAddrStrWithoutProvider = "EUcSN3d9TFETANxwEjpLW81M3hu8ftRfTh"
	contractAddrStrWithProvider    = "Eddc8R1rVTFQkDBYb471bYirqsz1h5L8mA"

	providerPK         = "09C73E3F79CFAA928089CD69AFCD5E1623B1D32415166A6A0436BF0FECAC9B7C"
	providerAddrStr    = "EcSFCKYh8Se2d3JnwaJmsaZtazTDAkKyLA"
	invadlidProviderPK = "5564a4ddd059ba6352aae637812ea6be7d818f92b5aff3564429478fcdfe4e8a"

	// payload to create a smart contract
	payload = "0x608060405260d0806100126000396000f30060806040526004361060525763ffffffff7c01000000000000000000000000000000000000000000000000000000006000350416633fb5c1cb811460545780638381f58a14605d578063f2c9ecd8146081575b005b60526004356093565b348015606857600080fd5b50606f6098565b60408051918252519081900360200190f35b348015608c57600080fd5b50606f609e565b600055565b60005481565b600054905600a165627a7a723058209573e4f95d10c1e123e905d720655593ca5220830db660f0641f3175c1cdb86e0029"

	providerWithoutGasAddr     = "ET7mfRv7D2cxEWGCPCjdcdCZZEF5k1ZSuh"
	providerWithoutGasPK       = "34b377a903b4a01c55062d978160084992271c4f89797caa18fd4e1d61123fbb"
	contractProviderWithoutGas = "Ecuu8zFTw8nWfJPxh4Vs6XNTJhzdw2RenS"

	senderWithoutGasPK      = "AEC5EB6A80CC094363D206949C3ED475C2C5060A23049150310D4FD39F95AF99"
	senderWithoutGasAddrStr = "EZksy7FEZJWS3ZsHfm5hVetB4CRG2ihicT"

	testGasLimit   = 1000000
	testGasPrice   = 1000000000
	testAmountSend = 1000000000
	ethRPCEndpoint = "http://localhost:22001"

	getReceiptMaxRetries = 20

	chainId = 15
)
