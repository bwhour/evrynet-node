// Copyright 2015 The go-ethereum Authors
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

package common

import (
	"database/sql/driver"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"
)

func TestBytesConversion(t *testing.T) {
	bytes := []byte{5}
	hash := BytesToHash(bytes)

	var exp Hash
	exp[31] = 5

	if hash != exp {
		t.Errorf("expected %x got %x", exp, hash)
	}
}

func TestIsHexAddress(t *testing.T) {
	tests := []struct {
		str string
		exp bool
	}{
		{"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed", true},
		{"5aaeb6053f3e94c9b9a09f33669435e7ef1beaed", true},
		{"0X5aaeb6053f3e94c9b9a09f33669435e7ef1beaed", true},
		{"0XAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", true},
		{"0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", true},
		{"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed1", false},
		{"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beae", false},
		{"5aaeb6053f3e94c9b9a09f33669435e7ef1beaed11", false},
		{"0xxaaeb6053f3e94c9b9a09f33669435e7ef1beaed", false},
	}

	for _, test := range tests {
		if result := IsHexAddress(test.str); result != test.exp {
			t.Errorf("IsHexAddress(%s) == %v; expected %v",
				test.str, result, test.exp)
		}
	}
}

func TestHexAddressHasUpperAndLowerCase(t *testing.T) {
	tests := []struct {
		str string
		exp string
	}{
		{"0xb61F4c3E676cE9f4FbF7f5597A303eEeC3AE531B", "0xb61F4c3E676cE9f4FbF7f5597A303eEeC3AE531B"},
		{"0xB61f4c3e676cE9f4FbF7f5597a303eeeC3ae531b", "0xb61F4c3E676cE9f4FbF7f5597A303eEeC3AE531B"},
	}

	for _, test := range tests {
		if HexToAddress(test.str).Hex() != test.exp {
			t.Errorf("Hex(%s) ; expected %v", HexToAddress(test.str).Hex(), test.exp)
		}
	}

}

func TestHashJsonValidation(t *testing.T) {
	var tests = []struct {
		Prefix string
		Size   int
		Error  string
	}{
		{"", 62, "json: cannot unmarshal hex string without 0x prefix into Go value of type common.Hash"},
		{"0x", 66, "hex string has length 66, want 64 for common.Hash"},
		{"0x", 63, "json: cannot unmarshal hex string of odd length into Go value of type common.Hash"},
		{"0x", 0, "hex string has length 0, want 64 for common.Hash"},
		{"0x", 64, ""},
		{"0X", 64, ""},
	}
	for _, test := range tests {
		input := `"` + test.Prefix + strings.Repeat("0", test.Size) + `"`
		var v Hash
		err := json.Unmarshal([]byte(input), &v)
		if err == nil {
			if test.Error != "" {
				t.Errorf("%s: error mismatch: have nil, want %q", input, test.Error)
			}
		} else {
			if err.Error() != test.Error {
				t.Errorf("%s: error mismatch: have %q, want %q", input, err, test.Error)
			}
		}
	}
}

func TestAddressUnmarshalJSON(t *testing.T) {
	var tests = []struct {
		Input     string
		ShouldErr bool
		Output    *big.Int
	}{
		{"", true, nil},
		{`""`, true, nil},
		{`"0x"`, true, nil},
		{`"0x00"`, true, nil},
		{`"0x0000000000000000000000000000000000000010"`, true, nil},
		{`"0x0000000000000000000000000000000000000000"`, true, nil},
		{`"EH9uVaqWRxHuzJbroqzX18yxmeW8XVJyV9d"`, true, nil},
		{`"EH9uVaqWRxHuzJbroqzX18yxmeW8XVJyV9"`, false, big.NewInt(0)},
		{`"EH9uVaqWRxHuzJbroqzX18yxmeWAGk5R72"`, false, big.NewInt(16)},
		{`"DspJWUYDimq3AsTmnRfCX1iB99FDc7eWAR"`, true, nil},
	}
	for i, test := range tests {
		var v Address
		err := json.Unmarshal([]byte(test.Input), &v)
		if err != nil && !test.ShouldErr {
			t.Errorf("test #%d: unexpected error: %v", i, err)
		}
		if err == nil {
			if test.ShouldErr {
				t.Errorf("test #%d: expected error, got none", i)
			}
			if got := new(big.Int).SetBytes(v.Bytes()); got.Cmp(test.Output) != 0 {
				t.Errorf("test #%d: address mismatch: have %v, want %v", i, got, test.Output)
			}
		}
	}
}

func TestAddressHexChecksum(t *testing.T) {
	var tests = []struct {
		Input  string
		Output string
	}{
		{"ERRPbyyv9bu3jxbvjuKM7T9c8dQTUYZs6b", "ERRPbyyv9bu3jxbvjuKM7T9c8dQTUYZs6b"},
		{"Eg5EzF235K8YkmWHfJJRQtZ6TR7kpSGacF", "Eg5EzF235K8YkmWHfJJRQtZ6TR7kpSGacF"},
		{"EdCqGAkqad1E6FoshjEGTjsUfzGDtpHcKi", "EdCqGAkqad1E6FoshjEGTjsUfzGDtpHcKi"},
		{"EcDhVmP9EthNpd8XTwvzQtG6sPV95EyCg6", "EcDhVmP9EthNpd8XTwvzQtG6sPV95EyCg6"},
		{"EH9uVaqWRxHuzJbroqzX18yxmeW9cqfaTq", "EH9uVaqWRxHuzJbroqzX18yxmeW9cqfaTq"},
	}
	for i, test := range tests {
		addr, err := EvryAddressStringToAddressCheck(test.Input)
		if err != nil {
			t.Errorf("test #%d: failed to decode %v", i, err)
		}
		output := AddressToEvryAddressString(addr)
		if output != test.Output {
			t.Errorf("test #%d: failed to match when it should (%s != %s)", i, output, test.Output)
		}
	}
}

func BenchmarkAddressHex(b *testing.B) {
	testAddr, _ := EvryAddressStringToAddressCheck("ERRPbyyv9bu3jxbvjuKM7T9c8dQTUYZs6b")
	for n := 0; n < b.N; n++ {
		testAddr.Hex()
	}
}

func TestMixedcaseAccount_Address(t *testing.T) {

	// https://github.com/ethereum/EIPs/blob/master/EIPS/eip-55.md
	// Note: 0X{checksum_addr} is not valid according to spec above

	var res []struct {
		A     MixedcaseAddress
		Valid bool
	}
	if err := json.Unmarshal([]byte(`[
		{"A" : "EZ53L1cPokpVFR2dsuRbSM58G9x74KDeQ4", "Valid": true},
		{"A" : "EJi9Rf88LxzNaQ9LbtFTrifRcEMw8hREvg", "Valid": true}
		]`), &res); err != nil {
		t.Fatal(err)
	}

	for _, r := range res {
		if got := r.A.ValidChecksum(); got != r.Valid {
			t.Errorf("Expected checksum %v, got checksum %v, input %v", r.Valid, got, r.A.String())
		}
	}

	//These should throw exceptions:
	var r2 []MixedcaseAddress
	for _, r := range []string{
		`["EZ53L1cPokpVFR2dsuRbSM58G9x74KDe"]`,        // Too short
		`["EZ53L1cPokpVFR2dsuRbSM58G9KDeQ4"]`,         // Too short
		`["EZ53L1cPokpVFR2dsuRbSM58G9x74KDeQ4ddd"]`,   // Too long
		`["EZ53L1cPokpVFR2dsuRbSM58G9x74KkdwefDeQ4"]`, // Too long
		`["EZ53L1cPokpVFR2dsuRbSM58G9x74KDOOeQ4"]`,    // wrong 'O'
		`["EZ53L1cPokpVFR2dsuRbSM58llG9x74KDOOeQ4"]`,  // wrong 'l'
		`["E9jSLuK76aMcRytYrV6GxDoLdehAKWJRfK"]`,      // wrong byte prefix 32
	} {
		if err := json.Unmarshal([]byte(r), &r2); err == nil {
			t.Errorf("Expected failure, input %v", r)
		}

	}

}

func TestHash_Scan(t *testing.T) {
	type args struct {
		src interface{}
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "working scan",
			args: args{src: []byte{
				0xb2, 0x6f, 0x2b, 0x34, 0x2a, 0xab, 0x24, 0xbc, 0xf6, 0x3e,
				0xa2, 0x18, 0xc6, 0xa9, 0x27, 0x4d, 0x30, 0xab, 0x9a, 0x15,
				0xa2, 0x18, 0xc6, 0xa9, 0x27, 0x4d, 0x30, 0xab, 0x9a, 0x15,
				0x10, 0x00,
			}},
			wantErr: false,
		},
		{
			name:    "non working scan",
			args:    args{src: int64(1234567890)},
			wantErr: true,
		},
		{
			name: "invalid length scan",
			args: args{src: []byte{
				0xb2, 0x6f, 0x2b, 0x34, 0x2a, 0xab, 0x24, 0xbc, 0xf6, 0x3e,
				0xa2, 0x18, 0xc6, 0xa9, 0x27, 0x4d, 0x30, 0xab, 0x9a, 0x15,
				0xa2, 0x18, 0xc6, 0xa9, 0x27, 0x4d, 0x30, 0xab, 0x9a, 0x15,
			}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Hash{}
			if err := h.Scan(tt.args.src); (err != nil) != tt.wantErr {
				t.Errorf("Hash.Scan() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				for i := range h {
					if h[i] != tt.args.src.([]byte)[i] {
						t.Errorf(
							"Hash.Scan() didn't scan the %d src correctly (have %X, want %X)",
							i, h[i], tt.args.src.([]byte)[i],
						)
					}
				}
			}
		})
	}
}

func TestHash_Value(t *testing.T) {
	b := []byte{
		0xb2, 0x6f, 0x2b, 0x34, 0x2a, 0xab, 0x24, 0xbc, 0xf6, 0x3e,
		0xa2, 0x18, 0xc6, 0xa9, 0x27, 0x4d, 0x30, 0xab, 0x9a, 0x15,
		0xa2, 0x18, 0xc6, 0xa9, 0x27, 0x4d, 0x30, 0xab, 0x9a, 0x15,
		0x10, 0x00,
	}
	var usedH Hash
	usedH.SetBytes(b)
	tests := []struct {
		name    string
		h       Hash
		want    driver.Value
		wantErr bool
	}{
		{
			name:    "Working value",
			h:       usedH,
			want:    b,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.h.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("Hash.Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Hash.Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddress_Scan(t *testing.T) {
	type args struct {
		src interface{}
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "working scan",
			args: args{src: []byte{
				0xb2, 0x6f, 0x2b, 0x34, 0x2a, 0xab, 0x24, 0xbc, 0xf6, 0x3e,
				0xa2, 0x18, 0xc6, 0xa9, 0x27, 0x4d, 0x30, 0xab, 0x9a, 0x15,
			}},
			wantErr: false,
		},
		{
			name:    "non working scan",
			args:    args{src: int64(1234567890)},
			wantErr: true,
		},
		{
			name: "invalid length scan",
			args: args{src: []byte{
				0xb2, 0x6f, 0x2b, 0x34, 0x2a, 0xab, 0x24, 0xbc, 0xf6, 0x3e,
				0xa2, 0x18, 0xc6, 0xa9, 0x27, 0x4d, 0x30, 0xab, 0x9a,
			}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Address{}
			if err := a.Scan(tt.args.src); (err != nil) != tt.wantErr {
				t.Errorf("Address.Scan() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				for i := range a {
					if a[i] != tt.args.src.([]byte)[i] {
						t.Errorf(
							"Address.Scan() didn't scan the %d src correctly (have %X, want %X)",
							i, a[i], tt.args.src.([]byte)[i],
						)
					}
				}
			}
		})
	}
}

func TestAddress_Value(t *testing.T) {
	b := []byte{
		0xb2, 0x6f, 0x2b, 0x34, 0x2a, 0xab, 0x24, 0xbc, 0xf6, 0x3e,
		0xa2, 0x18, 0xc6, 0xa9, 0x27, 0x4d, 0x30, 0xab, 0x9a, 0x15,
	}
	var usedA Address
	usedA.SetBytes(b)
	tests := []struct {
		name    string
		a       Address
		want    driver.Value
		wantErr bool
	}{
		{
			name:    "Working value",
			a:       usedA,
			want:    b,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.a.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("Address.Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Address.Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddressToEvryAddressString(t *testing.T) {
	var tests = []struct {
		Input    string
		Expect   string
		WantFail bool
	}{
		{"0x0000000000000000000000000000000000000000",  "EH9uVaqWRxHuzJbroqzX18yxmeW8XVJyV9", false},
		{"0x0000000000000000000000000000000000000011",  "EH9uVaqWRxHuzJbroqzX18yxmeWAULFYZT", false},
	}
	for i, test := range tests {
		address := HexToAddress(test.Input)
		evryAddressString := AddressToEvryAddressString(address)
		isEqual := strings.Compare(evryAddressString, test.Expect) == 0
		if isEqual == test.WantFail {
			t.Errorf("test #%d: unexpected, output: %s, expected: %s", i, evryAddressString, test.Expect)
		}

	}
}

func TestEvryAddressStringToAddress(t *testing.T) {
	var tests = []struct {
		EvryAddrssStr string
		Expect        *big.Int
		WantFail      bool
	}{
		{"EH9uVaqWRxHuzJbroqzX18yxmeW8XVJyV9",  new(big.Int).SetUint64(0), false},
		{"EH9uVaqWRxHuzJbroqzX18yxmeWAopEdUM",  new(big.Int).SetUint64(20), false},
		{"EH9uVaqWRxHuzJbroqzX18yxmeWBLHhJUd",  new(big.Int).SetUint64(25), false},
		{"EH9uVaqWRxHuzJbroqzX18yxmeWBLHhJUd",  new(big.Int).SetUint64(26), true},
	}
	for i, test := range tests {
		addr, err := EvryAddressStringToAddressCheck(test.EvryAddrssStr)
		if err != nil && !test.WantFail {
			t.Errorf("test #%d: unexpected, error: %v", i, err)
		}
		res := new(big.Int).SetBytes(addr.Bytes())
		isEqual := res.Cmp(test.Expect) == 0

		if isEqual == test.WantFail {
			t.Errorf("test #%d: unexpected, Output:%v, Expect:%v", i, res.Uint64(), test.Expect.Uint64())
		}

	}
}
