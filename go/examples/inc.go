package examples

import (
	"encoding/hex"
	"log"
)

func GetIncrementExample() Example {
	// An implementation of the increment function.
	code, err := hex.DecodeString("608060405234801561001057600080fd5b506004361061002b5760003560e01c8063dd5d521114610030575b600080fd5b61004a600480360381019061004591906100b7565b610060565b60405161005791906100f3565b60405180910390f35b600060018261006f919061013d565b9050919050565b600080fd5b600063ffffffff82169050919050565b6100948161007b565b811461009f57600080fd5b50565b6000813590506100b18161008b565b92915050565b6000602082840312156100cd576100cc610076565b5b60006100db848285016100a2565b91505092915050565b6100ed8161007b565b82525050565b600060208201905061010860008301846100e4565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b60006101488261007b565b91506101538361007b565b9250828201905063ffffffff81111561016f5761016e61010e565b5b9291505056fea26469706673582212209d38ad29206856b2e180ad12d1ab4e5f537d6cfa4e6f4138f0e9e3ff07f7925664736f6c637827302e382e31372d646576656c6f702e323032322e382e392b636f6d6d69742e62623161386466390058")
	if err != nil {
		log.Fatalf("Unable to decode increment-code: %v", err)
	}

	return exampleSpec{
		Name:      "inc",
		code:      code,
		function:  0xDD5D5211,
		reference: inc,
	}.build()
}

func inc(x int) int {
	return x + 1
}
