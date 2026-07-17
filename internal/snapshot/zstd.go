package snapshot

import (
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
)

// CompressDatabase creates a compacted zstd archive of the sqlite database
func CompressDatabase(sqlitePath string, zstdDestPath string) error {
	inputFile, err := os.Open(sqlitePath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	if err := os.MkdirAll(filepath.Dir(zstdDestPath), 0755); err != nil {
		return err
	}

	outputFile, err := os.Create(zstdDestPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	writer, err := zstd.NewWriter(outputFile, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = io.Copy(writer, inputFile)
	return err
}

// DecompressDatabase extracts the zstd archive to sqlite destination
func DecompressDatabase(zstdPath string, sqliteDestPath string) error {
	inputFile, err := os.Open(zstdPath)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	if err := os.MkdirAll(filepath.Dir(sqliteDestPath), 0755); err != nil {
		return err
	}

	reader, err := zstd.NewReader(inputFile)
	if err != nil {
		return err
	}
	defer reader.Close()

	outputFile, err := os.Create(sqliteDestPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, reader)
	return err
}
