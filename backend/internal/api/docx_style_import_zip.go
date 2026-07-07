package api

import (
	"archive/zip"
	"bytes"
)

func readDocxDocumentXMLFromBytes(docxBytes []byte) (string, bool) {
	reader, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		return "", false
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		raw, err := readZipFile(file)
		if err != nil {
			return "", false
		}
		return string(raw), true
	}
	return "", false
}
