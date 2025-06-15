package embedding

// Model represents an interface for text embedding
type Model interface {
	Encode(text string) ([]float32, error)
}

// SimpleModel is a basic implementation of Model
type SimpleModel struct {
	dimension int
}

// NewSimpleModel creates a new simple embedding model
func NewSimpleModel(dimension int) *SimpleModel {
	return &SimpleModel{
		dimension: dimension,
	}
}

// Encode implements the Model interface
func (m *SimpleModel) Encode(text string) ([]float32, error) {
	// This is a simple hash-based embedding for demonstration
	// In production, you would use a proper embedding model
	vector := make([]float32, m.dimension)
	hash := 0
	for _, c := range text {
		hash = 31*hash + int(c)
	}
	for i := range vector {
		vector[i] = float32(hash%100) / 100.0
	}
	return vector, nil
}
