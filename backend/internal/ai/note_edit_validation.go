package ai

import (
	"fmt"
	"strings"

	nethtml "golang.org/x/net/html"

	"webide-backend/internal/workspace"
)

type noteStructureReport struct {
	BeforeBlocks        int `json:"before_blocks"`
	AfterBlocks         int `json:"after_blocks"`
	EmptyListsBefore    int `json:"empty_lists_before"`
	EmptyListsAfter     int `json:"empty_lists_after"`
	EmptyItemsBefore    int `json:"empty_list_items_before"`
	EmptyItemsAfter     int `json:"empty_list_items_after"`
	InvalidTablesBefore int `json:"invalid_tables_before"`
	InvalidTablesAfter  int `json:"invalid_tables_after"`
	EmptyWrappersBefore int `json:"empty_wrappers_before"`
	EmptyWrappersAfter  int `json:"empty_wrappers_after"`
}

func validateNoteEditStructure(before, after string, edits []noteBlockEdit) (noteStructureReport, error) {
	if workspace.SanitizeHTMLFragment(after) != after ||
		workspace.SanitizeHTMLFragment(workspace.SanitizeHTMLFragment(after)) != after {
		return noteStructureReport{}, newToolError(ToolErrorVerificationFailed, "edited note sanitizer result is not stable", nil)
	}
	beforeRoot, beforeNodes, err := parsedNoteBlocks(before)
	if err != nil {
		return noteStructureReport{}, err
	}
	afterRoot, afterNodes, err := parsedNoteBlocks(after)
	if err != nil {
		return noteStructureReport{}, err
	}
	report := noteStructureReport{BeforeBlocks: len(beforeNodes), AfterBlocks: len(afterNodes)}
	report.EmptyListsBefore, report.EmptyItemsBefore, report.InvalidTablesBefore, report.EmptyWrappersBefore = noteInvalidStructureCounts(beforeRoot)
	report.EmptyListsAfter, report.EmptyItemsAfter, report.InvalidTablesAfter, report.EmptyWrappersAfter = noteInvalidStructureCounts(afterRoot)
	if report.EmptyListsAfter > report.EmptyListsBefore ||
		report.EmptyItemsAfter > report.EmptyItemsBefore ||
		report.InvalidTablesAfter > report.InvalidTablesBefore ||
		report.EmptyWrappersAfter > report.EmptyWrappersBefore {
		typed := newToolError(ToolErrorVerificationFailed, "edited note introduced empty or invalid HTML structure", nil)
		typed.Details = map[string]any{"structure_delta": report}
		return noteStructureReport{}, typed
	}

	changed := map[string]string{}
	for _, edit := range edits {
		startID := strings.TrimSpace(edit.BlockID)
		operation := strings.TrimSpace(edit.Operation)
		changed[startID] = operation
		if operation == "delete_block_range" {
			for ordinal := noteBlockOrdinal(startID); ordinal <= noteBlockOrdinal(edit.EndBlockID); ordinal++ {
				changed[fmt.Sprintf("b%04d", ordinal)] = operation
			}
		}
	}
	afterCanonical := map[string]int{}
	for _, node := range afterNodes {
		afterCanonical[renderHTMLNode(node)]++
	}
	for index, node := range beforeNodes {
		id := fmt.Sprintf("b%04d", index+1)
		switch changed[id] {
		case "delete", "delete_block_range", "replace_html", "replace_inner_html", "replace_text":
			continue
		}
		canonical := renderHTMLNode(node)
		if afterCanonical[canonical] == 0 {
			typed := newToolError(ToolErrorVerificationFailed, "an undeclared note block changed during the edit", nil)
			typed.Details = map[string]any{"block_id": id}
			return noteStructureReport{}, typed
		}
		afterCanonical[canonical]--
	}
	return report, nil
}

func parsedNoteBlocks(fragment string) (*nethtml.Node, []*nethtml.Node, error) {
	root, err := nethtml.Parse(strings.NewReader("<!doctype html><html><body>" + fragment + "</body></html>"))
	if err != nil {
		return nil, nil, newToolError(ToolErrorVerificationFailed, "note HTML could not be parsed for structural validation", err)
	}
	body := findFirstHTMLNode(root, "body")
	if body == nil {
		return nil, nil, newToolError(ToolErrorVerificationFailed, "note body is missing during structural validation", nil)
	}
	return body, collectNoteBlockNodes(body), nil
}

func noteInvalidStructureCounts(root *nethtml.Node) (emptyLists, emptyItems, invalidTables, emptyWrappers int) {
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node == nil {
			return
		}
		if node.Type == nethtml.ElementNode {
			switch strings.ToLower(node.Data) {
			case "ul", "ol":
				if !hasDirectElementChild(node, "li") {
					emptyLists++
				}
			case "li":
				if strings.TrimSpace(noteNodeText(node)) == "" && !hasMeaningfulElement(node) {
					emptyItems++
				}
			case "table":
				if !hasDescendantElement(node, "tr") || !hasDescendantElement(node, "td", "th") {
					invalidTables++
				}
			case "div", "span":
				if strings.TrimSpace(noteNodeText(node)) == "" && !hasMeaningfulElement(node) {
					emptyWrappers++
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return
}

func noteNodeText(root *nethtml.Node) string {
	var text strings.Builder
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.TextNode {
			text.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return text.String()
}

func hasDirectElementChild(root *nethtml.Node, tags ...string) bool {
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.ElementNode && containsFold(tags, child.Data) {
			return true
		}
	}
	return false
}

func hasDescendantElement(root *nethtml.Node, tags ...string) bool {
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.ElementNode && containsFold(tags, child.Data) {
			return true
		}
		if hasDescendantElement(child, tags...) {
			return true
		}
	}
	return false
}

func hasMeaningfulElement(root *nethtml.Node) bool {
	return hasDescendantElement(root, "img", "table", "hr", "br", "video", "audio", "iframe")
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
