package gen

import (
	"pgregory.net/rand"

	"github.com/Fantom-foundation/Tosca/go/ct/st"
)

type MemoryGenerator struct {
}

func NewMemoryGenerator() *MemoryGenerator {
	return &MemoryGenerator{}
}

func (g *MemoryGenerator) Generate(rnd *rand.Rand) (*st.Memory, error) {
	// Pick a size; since memory is always grown in 32 byte steps, we also
	// generate only memory segments where size is a multiple of 32.
	size := 32 * rnd.Intn(10)

	data := make([]byte, size)
	_, err := rnd.Read(data)
	if err != nil {
		return nil, err
	}

	return st.NewMemory(data...), nil
}

func (g *MemoryGenerator) Clone() *MemoryGenerator {
	return &MemoryGenerator{}
}

func (*MemoryGenerator) Restore(*MemoryGenerator) {
}

func (g *MemoryGenerator) String() string {
	return "{}"
}