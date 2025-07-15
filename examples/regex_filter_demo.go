package main

import (
	"fmt"
	"log"
	"time"

	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/internal/processor"
	"fsevents/pkg/types"
)

func main() {
	// Setup logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}

	// Define regex filters
	filters := []config.FilterRule{
		{
			Field:    "Caller-Caller-ID-Number",
			Operator: "regex",
			Value:    "^\\+?1?[2-9]\\d{2}[2-9]\\d{2}\\d{4}$", // US phone number format
		},
		{
			Field:    "variable_sip_user_agent",
			Operator: "regex",
			Value:    "(?i)polycom|cisco|yealink", // Allow known good devices
		},
	}

	// Create event filter
	eventFilter := processor.NewEventFilter(filters, logger)

	// Test events
	testEvents := []*types.Event{
		{
			Name:      "CHANNEL_CREATE",
			Timestamp: time.Now(),
			Headers: map[string]string{
				"Caller-Caller-ID-Number": "+12125551234",
				"variable_sip_user_agent": "Polycom SoundPoint IP",
				"Call-Direction":          "inbound",
			},
		},
		{
			Name:      "CHANNEL_CREATE",
			Timestamp: time.Now(),
			Headers: map[string]string{
				"Caller-Caller-ID-Number": "invalid-phone",
				"variable_sip_user_agent": "Cisco SPA525G",
				"Call-Direction":          "inbound",
			},
		},
		{
			Name:      "CHANNEL_CREATE",
			Timestamp: time.Now(),
			Headers: map[string]string{
				"Caller-Caller-ID-Number": "+19175551234",
				"variable_sip_user_agent": "Evil Bot Scanner",
				"Call-Direction":          "inbound",
			},
		},
	}

	fmt.Println("=== Regex Filter Demo ===")
	fmt.Println()

	for i, event := range testEvents {
		fmt.Printf("Test Event %d:\n", i+1)
		fmt.Printf("  Caller-ID: %s\n", event.GetHeader("Caller-Caller-ID-Number"))
		fmt.Printf("  User-Agent: %s\n", event.GetHeader("variable_sip_user_agent"))

		shouldProcess := eventFilter.ShouldProcess(event)

		if shouldProcess {
			fmt.Printf("  ✅ PASSED - Event will be processed\n")
		} else {
			fmt.Printf("  ❌ FILTERED - Event will be rejected\n")
		}
		fmt.Println()
	}

	fmt.Println("=== Individual Regex Pattern Tests ===")
	fmt.Println()

	// Test individual patterns
	patterns := map[string]string{
		"US Phone Pattern":  "^\\+?1?[2-9]\\d{2}[2-9]\\d{2}\\d{4}$",
		"UUID Pattern":      "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
		"Device Pattern":    "(?i)polycom|cisco|yealink",
		"Direction Pattern": "^(inbound|outbound)$",
	}

	testValues := map[string][]string{
		"US Phone Pattern": {
			"+12125551234",   // Valid
			"2125551234",     // Valid
			"+1234567890",    // Invalid (starts with 1)
			"invalid-number", // Invalid
		},
		"UUID Pattern": {
			"12345678-1234-5678-9abc-def012345678", // Valid
			"invalid-uuid-format",                  // Invalid
		},
		"Device Pattern": {
			"Polycom SoundPoint IP", // Valid
			"Cisco SPA525G",         // Valid
			"Evil Bot Scanner",      // Invalid
		},
		"Direction Pattern": {
			"inbound",           // Valid
			"outbound",          // Valid
			"invalid-direction", // Invalid
		},
	}

	for patternName, pattern := range patterns {
		fmt.Printf("%s: %s\n", patternName, pattern)

		for _, testValue := range testValues[patternName] {
			filter := processor.NewEventFilter([]config.FilterRule{{
				Field:    "test_field",
				Operator: "regex",
				Value:    pattern,
			}}, logger)

			testEvent := &types.Event{
				Name:      "TEST",
				Timestamp: time.Now(),
				Headers: map[string]string{
					"test_field": testValue,
				},
			}

			matches := filter.ShouldProcess(testEvent)
			if matches {
				fmt.Printf("  ✅ \"%s\" matches\n", testValue)
			} else {
				fmt.Printf("  ❌ \"%s\" does not match\n", testValue)
			}
		}
		fmt.Println()
	}
}
