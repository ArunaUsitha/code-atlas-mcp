package search

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

type LocalEmbedder struct {
	session *ort.DynamicAdvancedSession
	enabled bool
}

func NewLocalEmbedder(modelPath string, dllPath string) (*LocalEmbedder, error) {
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return &LocalEmbedder{enabled: false}, fmt.Errorf("ONNX model file not found: %s", modelPath)
	}

	if dllPath == "" {
		dllPath = "onnxruntime.dll" // Default for Windows
	}

	ort.SetSharedLibraryPath(dllPath)
	err := ort.InitializeEnvironment()
	if err != nil {
		return &LocalEmbedder{enabled: false}, fmt.Errorf("failed to initialize ONNX environment: %w", err)
	}

	session, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"output"},
		nil)
	if err != nil {
		return &LocalEmbedder{enabled: false}, fmt.Errorf("failed to load ONNX session: %w", err)
	}

	return &LocalEmbedder{
		session: session,
		enabled: true,
	}, nil
}

func (le *LocalEmbedder) IsEnabled() bool {
	return le.enabled
}

// GenerateEmbeddings converts text into float32 array
func (le *LocalEmbedder) GenerateEmbeddings(text string) ([]float32, error) {
	if !le.enabled {
		// Fallback to simple hash-based vector generation (Mock Embedder)
		return GenerateMockEmbedding(text), nil
	}

	// Simple tokenizer: split by spaces and map to arbitrary int64 IDs
	words := strings.Fields(strings.ToLower(text))
	tokens := make([]int64, len(words))
	attentionMask := make([]int64, len(words))

	for i, w := range words {
		tokens[i] = int64(hashStringToInt(w))
		attentionMask[i] = 1
	}

	if len(tokens) == 0 {
		tokens = []int64{0}
		attentionMask = []int64{1}
	}

	inputShape := ort.NewShape(1, int64(len(tokens)))

	inputTensor, err := ort.NewTensor(inputShape, tokens)
	if err != nil {
		return nil, err
	}
	defer inputTensor.Destroy()

	maskTensor, err := ort.NewTensor(inputShape, attentionMask)
	if err != nil {
		return nil, err
	}
	defer maskTensor.Destroy()

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 768))
	if err != nil {
		return nil, err
	}
	defer outputTensor.Destroy()

	err = le.session.Run(
		[]ort.ArbitraryTensor{inputTensor, maskTensor},
		[]ort.ArbitraryTensor{outputTensor},
	)
	if err != nil {
		return nil, fmt.Errorf("ONNX run failed: %w", err)
	}

	return outputTensor.GetData(), nil
}

func hashStringToInt(s string) int {
	h := sha256.Sum256([]byte(s))
	return int(binary.BigEndian.Uint32(h[:4])) % 30522 // Vocab size scale
}

// GenerateMockEmbedding creates a deterministic, normalized 768-dim vector from text using hash hashing
func GenerateMockEmbedding(text string) []float32 {
	vector := make([]float32, 768)
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		words = []string{"empty"}
	}

	for _, word := range words {
		h := sha256.Sum256([]byte(word))
		// Distribute word hashes across vector indices
		for i := 0; i < 24; i++ {
			val := int(binary.BigEndian.Uint32(h[i%4*4 : i%4*4+4]))
			idx := val % 768
			// Generate positive/negative weights based on other bits
			weight := float32((val>>16)%100) / 50.0 // range -2.0 to 2.0
			if val%2 == 0 {
				vector[idx] += weight
			} else {
				vector[idx] -= weight
			}
		}
	}

	// Normalize vector (L2 norm)
	var norm float32
	for _, val := range vector {
		norm += val * val
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm > 0 {
		for i := range vector {
			vector[i] /= norm
		}
	} else {
		vector[0] = 1.0 // Unit vector fallback
	}

	return vector
}

// CosineSimilarity calculates the similarity metric between two float32 slices
func CosineSimilarity(v1, v2 []float32) float64 {
	if len(v1) != len(v2) || len(v1) == 0 {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := 0; i < len(v1); i++ {
		dotProduct += float64(v1[i] * v2[i])
		normA += float64(v1[i] * v1[i])
		normB += float64(v2[i] * v2[i])
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
