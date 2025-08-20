package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"strings"
	"time"
)

// Model defines the interface for embedding models
type Model interface {
	// Encode converts text to a vector embedding
	Encode(text string) ([]float32, error)
}

// SimpleModel implements a simple hash-based embedding model
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
	// Create a deterministic vector based on the hash of the text
	h := fnv.New32a()
	h.Write([]byte(text))
	hash := h.Sum32()

	// Generate a vector of the specified dimension
	vector := make([]float32, m.dimension)
	for i := range vector {
		// Use different parts of the hash for each dimension
		hash = hash*31 + uint32(i)
		vector[i] = float32(math.Sin(float64(hash)))
	}

	return vector, nil
}

// OpenAIModel implements an OpenAI-based embedding model
type OpenAIModel struct {
	apiKey    string
	model     string
	dimension int
}

// NewOpenAIModel creates a new OpenAI embedding model
func NewOpenAIModel(apiKey, model string, dimension int) *OpenAIModel {
	return &OpenAIModel{
		apiKey:    apiKey,
		model:     model,
		dimension: dimension,
	}
}

// Encode implements the Model interface
func (m *OpenAIModel) Encode(text string) ([]float32, error) {
	// TODO: Implement OpenAI API call
	// For now, use the simple model as a fallback
	simpleModel := NewSimpleModel(m.dimension)
	return simpleModel.Encode(text)
}

// SentenceTransformersModel implements a Sentence Transformers-based embedding model
type SentenceTransformersModel struct {
	model     string
	device    string
	dimension int
}

// NewSentenceTransformersModel creates a new Sentence Transformers embedding model
func NewSentenceTransformersModel(model, device string, dimension int) *SentenceTransformersModel {
	return &SentenceTransformersModel{
		model:     model,
		device:    device,
		dimension: dimension,
	}
}

// Encode implements the Model interface
func (m *SentenceTransformersModel) Encode(text string) ([]float32, error) {
	// TODO: Implement Sentence Transformers API call
	// For now, use the simple model as a fallback
	simpleModel := NewSimpleModel(m.dimension)
	return simpleModel.Encode(text)
}

// OllamaModel implements an Ollama-based embedding model
type OllamaModel struct {
	url       string
	model     string
	dimension int
	client    *http.Client
}

// NewOllamaModel creates a new Ollama embedding model
func NewOllamaModel(url, model string, dimension int) *OllamaModel {
	return &OllamaModel{
		url:       url,
		model:     model,
		dimension: dimension,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Encode implements the Model interface
func (m *OllamaModel) Encode(text string) ([]float32, error) {
	// Handle edge case of empty text
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("cannot generate embedding for empty text")
	}

	// Prepare the request payload
	payload := map[string]interface{}{
		"model":  m.model,
		"prompt": text,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %v", err)
	}

	// Make the API request
	resp, err := m.client.Post(m.url+"/api/embeddings", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to make Ollama API request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned status %d", resp.StatusCode)
	}

	// Parse the response
	var response struct {
		Embedding []float32 `json:"embedding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama API response: %v", err)
	}

	// Validate embedding dimension
	if len(response.Embedding) != m.dimension {
		return nil, fmt.Errorf("expected embedding dimension %d, got %d (text length: %d, text preview: '%.100s')",
			m.dimension, len(response.Embedding), len(text), text)
	}

	return response.Embedding, nil
}
