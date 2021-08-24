// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"math/big"

	"github.com/Evrynetlabs/evrynet-node/common"
)

// DAOForkBlockExtra is the block header extra-data field to set for the DAO fork
// point and a number of consecutive blocks to allow fast/light syncers to correctly
// pick the side they want  ("dao-hard-fork").
var DAOForkBlockExtra = common.FromHex("0x64616f2d686172642d666f726b")

// DAOForkExtraRange is the number of consecutive blocks from the DAO fork point
// to override the extra-data in to prevent no-fork attacks.
var DAOForkExtraRange = big.NewInt(10)

// DAORefundContract is the address of the refund contract to send DAO balances to.
var DAORefundContract, _ = common.EvryAddressStringToAddressCheck("EabT1JSGYFVVJqZepNmnzEi2yhoNc7YTw2")

// DAODrainList is the list of accounts whose full balances will be moved into a
// refund contract at the beginning of the dao-fork block.
func DAODrainList() []common.Address {
	addrStrs := []string{
		"Eca7c58exwmZqdHFTvi5wfmYAV7Gxafbo1",
		"EZZZCbxXVVB2noeefPNPbdEFoJgD1EiEHL",
		"EMB67L8iKVEY8jRqs1xU8tTkx9DWcx1uFQ",
		"Eek5b5eUiw1GVvw22pQipSGmvYF29Yivw9",
		"EKUXRzAWD37Hs9RrXjNF1k82csGVrixpNN",
		"EY5LpkFinEqNwiUx9WvsjNtR93kQfZt6m9",
		"EMgHc6NyBdyUqgNqoJnG6cVZs8FnkYHneY",
		"EHjxESUorzb3y4BgFUhpqDymr6HskoxfdQ",
		"ERb7F35X9Vdvxp4ewoEGYpwMmzHH2rGr8p",
		"ESmDSEeB7kMYYRByoEuJwNo7iYRvxRD1nR",
		"EU4CiU6ZurTbiUSmbuFitZJyEQnBP9jTG7",
		"EM1wVsdfcdzM9jZqbn6D4qr7zy92qErr63",
		"ERadvLcwwa1UjeHBkeHXhKcHvzTmDa28JY",
		"EXQR42xxLnGemkuQBuPX3qV2879abv6Ndq",
		"EL5CHceSa82PwWqbzYHpgBEdFbfCN5k8v6",
		"EaXHUi1YSWvnfdmBXKX32wTrERL1Dir894",
		"ESuvXrRmaTHqSyeGuTiJLW36mZBedtxn8Z",
		"Ef9MsjgELVujcVQKCUbF1deaS65dQoULxt",
		"EcGZRCra8riaPAHxfJRvG8USrhtTKFjK7G",
		"EUx4Lw5ngUrrpX5UZsSW6EJ8LhdkCXTQAL",
		"EQcqc8NLZfdFbjXPxqraysZZX5W4iRxUaL",
		"ES8buxwnkiP356gQ3mCnFAtS7oJuKrpdyG",
		"Ef6aVxKSPebrb4kJryEyv2PTq1qnP2UW7X",
		"ELWKdnpR6h2HEteeDAZ8S3LMo5zzVNaLES",
		"EXfSgGYxr9fxnq6mUaBqEyPJtxVywDyVKZ",
		"EYGtxkPoEmVHn29dmgcj1BgYTmz6ztry3E",
		"Eczui4tXrKjFePT5R5E1WbYoCVqgW4gADP",
		"ESGGj6KrcFBzWgZD5YfZdcgZDSzBxFso75",
		"ETJwumj3zY79atnbnqu2edTod57ewwWGwo",
		"EU5JU61bta4jG2MGAhbaW88oGxRpUSWPK6",
		"EPprhAMnmSfrjN2CE2ZJSYkgHMgo2ahAVu",
		"EJcEo3RCCMXnjB7EaiXJRqQ3mCeiiRSNVY",
		"EXcntMvcsb6DTNzriVsFRtQDj8qJyakuPZ",
		"Ebzjypmgn114c7sr11prVxEfgjgUADbu6F",
		"Ebme7m43WggMXNKRdoBY3AgNtvp6eqRybY",
		"ER97NMMXmMnySeoVwJzNQq3YTaE4CAQjH7",
		"EdrMMJcef19dQMMEf7z5omZnqzrPhP7kC2",
		"EQ9TPKDMN3BXHmmJewHc3sP2JCYqcZLjvc",
		"EYqzc2iyzXnXGUdZx9gnYonLhQyxzrmyzw",
		"EQS43eW1uAe2vKG1aRX71N9B5AhPt2QAsy",
		"EPMiEkPDQDxzbvo6hzuU984q57D5WTK1Ee",
		"EPQEjaZEvCWbsYdDbVfvm59BCmgfzguZ2S",
		"EXPCupNWE9cZHPyxxcFsfvVmRwiKjV2z71",
		"ELmn9M6TSahCce8f6VgwuYSptmSUPbHT3a",
		"ELEXNYLYrnSqUKMA6RTdH4DcLUopbVcqTF",
		"ELQkoP4md3S3V1o5sVAvJ7dBKdWbfoE3eC",
		"EKmQ5UGA9M3A7uQ8UAmhqjXY2GAjrMt7RP",
		"Ea49dUVsRCPNF92yjpZpoHNJiSTa6zGhCq",
		"ES1pf22ae6fF187sjvkLsKHzSN9k99LqoS",
		"EK7xZoRQBStLNFRC3dHzf8SG2k6EUvYcWJ",
		"EQpwGNWqHjHRN4UCd9ZazQTXzB4nKTMdL2",
		"Eb68vkGEGYpdp43mqv9U8F7pZLS7wPEErD",
		"EU7AQZbBbP5Zx547Apw52ai3RwD4agAct7",
		"ERF31dAbnsiJUpgfyi5NM2LUYXjrvT9gg1",
		"ENbGqUgyGsm2oum74yU1fJavf58GdSER4N",
		"EZfcSq82KTDEprnK35WHEtnyThM5a8qLR2",
		"Ee149TvpMoN7itP9Ch8TWa29JxXgnrpr2P",
		"ENdDDagCMRk8Wi4vMAP1WXgYYG141CHA7g",
		"EHp4teH95NrBD5jQWE2kgK4wk3wGRKJG44",
		"EX1N6mZ5uUhFhEWuh7nmt545k6dRFS2DAf",
		"EQhZ8Yb2AvTZwhs3Uk1wWXUm2JfBAGcr4a",
		"EPkeuNfNT9n49LGNmCmaGJpJ7tXhbWSYBf",
		"EReY4ij9QMqkLESCaSyk8yb7JzzHn5gpxr",
		"ERsWQKV1QGtkidST7jbiHVHis4hkAwCxaE",
		"EHetdCNdJZ4buRoYTWXBwkk6Q5ocfN39Jt",
		"EXFVGZ47ZppF1NGWt2w4Tj25DGrfkfc9jB",
		"EMZEyYu8AdF5gTnmZJuctprGNDJ9qmGJQU",
		"EWQBqvqvGSugL1RMQHDfBXDNLooJeaq2pc",
		"EQFu3Vjc1jCuqQNZHMoES8BucAdnfV9r1g",
		"EHszdStHEK856uwGCk25QRrULdu2jBJWRF",
		"EN3TMu9yTNzwfZvg3D4AjggqGfTBiWBXrz",
		"EQR69BNRy7fxNPpQMLJ4E1MbbMp9BnceXD",
		"EXXRZKXhHaBXVSbCJhjRzJoCPMu8ALDS5X",
		"EW4j59wg5i7ndtdGJk2iVNQ2Ww5PzLvSEA",
		"EHFEWQrjtc7suWqhYTGn5Ee6PbCwW5MTTo",
		"ERhfPXhP4jxoAMyqJcQa8ZjKxbQf7DWu2T",
		"EaP6Lm68tNSJ7xyKw1muCQ782ePSZ5q6zT",
		"EJsqNDdKtdwVjwUwU4cVihqHEXHcreKffe",
		"EY1UdCYrtm5Zp9aupT3pQHSvM3xkk31tke",
		"EeihUvgaCt4tnuSgrUeBmbppnbEaewc5ck",
		"EcF5LiF1ZeXxNhbesoTTxDfe6T6wmFDjki",
		"ES77GagMNoYgoyMks4VmjTtoiR89sYEvU7",
		"EKmoQBhtZZbir5bhfBC27dVVS4wRs946G8",
		"EYVBa6YMSx5aXzRb2pez1SJnjdEsvYfe6z",
		"EXirjCQvwCoZE69NUDVMdTvkei2hqKmXgq",
		"EHBsjXGAfoJCzsYp4YQFzX4Ni4xLfLYbt9",
		"EJSDJrtrRcmJ4eo2jwangkraNCmLjEYq9Y",
		"EcE1tFyY3rrwceVD2Kwr3jDKqN6k8XULhG",
		"EaJ6zn8HH2PcbuVjy8uNJUjJSUKfNDN5SH",
		"EPi6zbcAiPcNvfRFSjYHnnqaJjGo26Ne84",
		"EYuq4mDDP5dGFJYLrVoqmRQSNrHtAFDM6J",
		"EZ1mYptvGBgGPqAf1RaFVJBhW9pHA2yLvE",
		"EQv6ynNTH3zMwR4Wsznw3ebyWnMxXX9m47",
		"EP47BQ8VReFdKCMkAXdu9MVrcUomjYPZhf",
		"EgKhDW3per68YvfwR3t1aqYNaPsevVGNsS",
		"EKJAbRKudWzQWQFaayaAyegzhNoABFiwsg",
		"ELYdRdDHhur4AnEwoPT5XhYHSQZe7RNikD",
		"EVS3yVLKJvTeZSeD64TNTgLjgJwJpVKB2L",
		"EZJvK3hd8Drxs8KPZkpJCVs11hBd3yfH6k",
		"EdChT8zanZKryUVLjWYqfhKBczYLMrBMe1",
		"Ef9mUpjyTcqULTVen71fGeJdCxtKYML1Fa",
		"Ebbiz2MWaR74JwLKX7s6KAYhMnudJusqXV",
		"EZ6oG7nQT7WTRk8vfgP8whQ9bVa6i8ej9k",
		"Ebj7GwMAnDzvjzpVWzToSEXVahVwwGZhgL",
		"ET93HD6NNvpv87J9tMZ3w5HtbuqFszhEvK",
		"EZTC6PQFtvKSagxuDG26ZdA1WQxVetDrpD",
		"EYuaGGAiStWFAMmdALyJgZsxjGZ31QKAVf",
		"EM6MFBDRSALETpcQnBpwprr1nnuquqmUju",
		"EPYShJqZB1dx7SyNjD9if7GGEf5Gphzo4f",
		"EcQyAdzA2PZVAwA2NBx2xFCxcyxfB81851",
		"EVGoMJwsrdiheFqbnWT9M8RJZNuNKqcRkp",
		"Ed3aE4B4pkzyAWvXMGdY9M93S7kLwNZhy4",
		"EfU9tReaHgA66wHKkt31VdpnkxQaqRTt3Q",
		"ETutU9qa89k8D1j3W4kqh3shkCzZGGsQVe",
		"EaFtSmjKCmztfUh24Fa56SBrze8mkj2J9Z",
		"EUs9eTqH85MJzs9UVAo9maLPHeFUaS4nif",
	}
	addresses := make([]common.Address, len(addrStrs))
	for i, v := range addrStrs {
		addresses[i], _ = common.EvryAddressStringToAddressCheck(v)
	}
	return addresses
}
