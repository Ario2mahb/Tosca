//
// Copyright (c) 2024 Fantom Foundation
//
// Use of this software is governed by the Business Source License included
// in the LICENSE file and at fantom.foundation/bsl11.
//
// Change Date: 2028-4-16
//
// On the date above, in accordance with the Business Source License, use
// of this software will be governed by the GNU Lesser General Public Licence v3
//

package vm_test

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/params"

	// This is only imported to get the EVM opcode definitions.
	// TODO: write up our own op-code definition and remove this dependency.
	evm "github.com/ethereum/go-ethereum/core/vm"
)

func TestStackMaxBoundary(t *testing.T) {
	// For every variant of interpreter
	for _, variant := range Variants {
		for _, revision := range revisions {
			for op, info := range getInstructions(revision) {
				if info.stack.popped >= info.stack.pushed {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/%s", variant, revision, op), func(t *testing.T) {
					evm := GetCleanEVM(revision, variant, nil)

					// push size
					size := info.stack.pushed - info.stack.popped
					// needed stack size
					size = int(params.StackLimit) - size + 1

					code := getCode(size, op)

					// Run an interpreter
					res, err := evm.Run(code, []byte{})

					// Check the result.
					if err != nil {
						t.Fatalf("unexpected error during EVM execution: %v", err)
					}
					if res.Success {
						t.Errorf("execution should have failed due to a stack overflow, got result %v", res)
					}
					// Note: the amount of consumed gas is not relevant, since
					// in case of a stack overflow all remaining gas is consumed.
				})
			}
		}
	}
}

func TestStackMinBoundary(t *testing.T) {
	// For every variant of interpreter
	for _, variant := range Variants {
		for _, revision := range revisions {
			for op, info := range getInstructions(revision) {
				if info.stack.popped <= 0 {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/%s", variant, revision, op), func(t *testing.T) {
					evm := GetCleanEVM(revision, variant, nil)
					code := getCode(info.stack.popped-1, op)

					// Run an interpreter
					res, err := evm.Run(code, []byte{})

					// Check the result.
					if err != nil {
						t.Fatalf("unexpected error during EVM execution: %v", err)
					}
					if res.Success {
						t.Errorf("execution should have failed due to a stack underflow, got result %v", res)
					}
					// Note: the amount of consumed gas is not relevant, since
					// in case of a stack underflow all remaining gas is consumed.
				})
			}
		}
	}
}

func getCode(stackLength int, op evm.OpCode) []byte {
	code := make([]byte, 0, stackLength*2+1)

	// Add to stack PUSH1 instructions
	for i := 0; i < stackLength; i++ {
		code = append(code, []byte{byte(evm.PUSH1), byte(0)}...)
	}

	// Set a tested instruction as the last one.
	code = append(code, byte(op))
	return code
}
