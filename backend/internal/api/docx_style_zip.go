package api

import (
	"archive/zip"
	"bytes"
	"io"
)

func rewriteDocxDocumentXML(docxBytes []byte, transform func(string) string) ([]byte, bool) {
	reader, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		return nil, false
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	foundDocument := false
	for _, file := range reader.File {
		header := file.FileHeader
		header.CRC32 = 0
		header.CompressedSize = 0
		header.UncompressedSize = 0
		header.CompressedSize64 = 0
		header.UncompressedSize64 = 0
		entry, err := writer.CreateHeader(&header)
		if err != nil {
			_ = writer.Close()
			return nil, false
		}
		content, err := readZipFile(file)
		if err != nil {
			_ = writer.Close()
			return nil, false
		}
		if file.Name == "word/document.xml" {
			foundDocument = true
			content = []byte(transform(string(content)))
		}
		if _, err := entry.Write(content); err != nil {
			_ = writer.Close()
			return nil, false
		}
	}
	if err := writer.Close(); err != nil {
		return nil, false
	}
	if !foundDocument {
		return nil, false
	}
	return buffer.Bytes(), true
}

func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
