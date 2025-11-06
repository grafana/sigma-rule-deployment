package querytest

import (
	"fmt"

	"github.com/grafana/sigma-rule-deployment/internal/model"
)

// ProcessFrame processes a single frame from the query response and updates the result stats
func ProcessFrame(frame model.Frame, result *model.QueryTestResult, showSampleValues, showLogLines bool) error {
	// Map field names to their indices
	fieldIndices := make(map[string]int)
	for i, field := range frame.Schema.Fields {
		fieldIndices[field.Name] = i
	}

	// Skip if no values
	if len(frame.Data.Values) == 0 {
		return nil
	}

	// Get the number of rows from the first field's values
	numRows := 0
	for _, values := range frame.Data.Values {
		if len(values) > numRows {
			numRows = len(values)
		}
	}

	// Process each row of values
	for rowIndex := 0; rowIndex < numRows; rowIndex++ {
		// Process labels if present
		if labelIndex, ok := fieldIndices["labels"]; ok {
			if labelIndex < len(frame.Data.Values) {
				if rowIndex < len(frame.Data.Values[labelIndex]) {
					if labelValues, ok := frame.Data.Values[labelIndex][rowIndex].(map[string]any); ok {
						for label, value := range labelValues {
							if _, exists := result.Stats.Fields[label]; !exists {
								if showSampleValues {
									result.Stats.Fields[label] = fmt.Sprintf("%v", value)
								} else {
									result.Stats.Fields[label] = ""
								}
							}
						}
					}
				}
			}
		}

		// Process Line field if present
		if lineIndex, ok := fieldIndices["Line"]; ok {
			if lineIndex < len(frame.Data.Values) {
				if rowIndex < len(frame.Data.Values[lineIndex]) {
					if lineValue, ok := frame.Data.Values[lineIndex][rowIndex].(string); ok {
						result.Stats.Count++
						// Only store the line value if show_log_lines is enabled
						if showLogLines {
							if _, exists := result.Stats.Fields["Line"]; !exists {
								result.Stats.Fields["Line"] = lineValue
							}
						}
					}
				}
			}
		}
	}
	return nil
}
