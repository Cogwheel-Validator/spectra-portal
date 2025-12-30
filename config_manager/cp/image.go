package cp

// Package cp provides functions to copy images to the public/icons directory
// within the frontend project

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyChainImages copies all images for a chain from the source directory
// to the destination directory. It maps from registry name (e.g., "osmosis")
// to chain ID (e.g., "osmosis-1") for the destination path.
func CopyChainImages(imagesDir, publicDir string, registryName string) error {
	// Build source path: images/{registry}/
	srcPath := filepath.Join(imagesDir, registryName)

	// Check if source directory exists
	srcInfo, err := os.Stat(srcPath)
	if os.IsNotExist(err) {
		// Source doesn't exist, skip silently (chain might not have images)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to stat source directory %s: %w", srcPath, err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source path is not a directory: %s", srcPath)
	}

	// Build destination path: public/icons/{chain_id}/
	destPath := filepath.Join(publicDir, "icons", registryName)

	// Create destination directory
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", destPath, err)
	}

	// Read all files in source directory
	entries, err := os.ReadDir(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source directory %s: %w", srcPath, err)
	}

	// Copy each file
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories
		}

		srcFile := filepath.Join(srcPath, entry.Name())
		destFile := filepath.Join(destPath, entry.Name())

		if err := copyFile(srcFile, destFile); err != nil {
			return fmt.Errorf("failed to copy %s to %s: %w", srcFile, destFile, err)
		}
	}

	return nil
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
