package richerrors

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestRichErrorsScenarios(t *testing.T) {
	t.Run("JSON Marshaling", func(t *testing.T) {
		// Test with both ExternalMsg and Err set
		err1 := Error{
			Code:        404,
			ExternalMsg: "User not found",
			Err:         fmt.Errorf("database query failed: user_id=123 not found"),
		}

		json1, err := json.Marshal(err1)
		if err != nil {
			t.Fatalf("Failed to marshal err1: %v", err)
		}
		t.Logf("JSON marshaled (both fields set): %s", string(json1))

		// Test with only ExternalMsg set
		err2 := Error{
			Code:        500,
			ExternalMsg: "Internal server error",
		}

		json2, err := json.Marshal(err2)
		if err != nil {
			t.Fatalf("Failed to marshal err2: %v", err)
		}
		t.Logf("JSON marshaled (only ExternalMsg): %s", string(json2))

		// Test with only Err set
		err3 := Error{
			Code: 400,
			Err:  fmt.Errorf("validation failed: email is invalid"),
		}

		json3, err := json.Marshal(err3)
		if err != nil {
			t.Fatalf("Failed to marshal err3: %v", err)
		}
		t.Logf("JSON marshaled (only Err): %s", string(json3))

		// Test with neither set
		err4 := Error{
			Code: 200,
		}

		json4, err := json.Marshal(err4)
		if err != nil {
			t.Fatalf("Failed to marshal err4: %v", err)
		}
		t.Logf("JSON marshaled (neither field set): %s", string(json4))
	})

	t.Run("Sprintf Formatting", func(t *testing.T) {
		// Test with both fields set
		err1 := Error{
			Code:        404,
			ExternalMsg: "User not found",
			Err:         fmt.Errorf("database query failed: user_id=123 not found"),
		}

		t.Logf("Sprintf %%v (both fields): %v", err1)
		t.Logf("Sprintf %%s (both fields): %s", err1)
		t.Logf("Sprintf %%+v (both fields): %+v", err1)

		// Test with only ExternalMsg set
		err2 := Error{
			Code:        500,
			ExternalMsg: "Internal server error",
		}

		t.Logf("Sprintf %%v (only ExternalMsg): %v", err2)
		t.Logf("Sprintf %%s (only ExternalMsg): %s", err2)
		t.Logf("Sprintf %%+v (only ExternalMsg): %+v", err2)

		// Test with only Err set
		err3 := Error{
			Code: 400,
			Err:  fmt.Errorf("validation failed: email is invalid"),
		}

		t.Logf("Sprintf %%v (only Err): %v", err3)
		t.Logf("Sprintf %%s (only Err): %s", err3)
		t.Logf("Sprintf %%+v (only Err): %+v", err3)

		// Test with neither set
		err4 := Error{
			Code: 200,
		}

		t.Logf("Sprintf %%v (neither field): %v", err4)
		t.Logf("Sprintf %%s (neither field): %s", err4)
		t.Logf("Sprintf %%+v (neither field): %+v", err4)
	})

	t.Run("Direct Method Calls", func(t *testing.T) {
		// Test with both fields set
		err1 := Error{
			Code:        404,
			ExternalMsg: "User not found",
			Err:         fmt.Errorf("database query failed: user_id=123 not found"),
		}

		t.Logf("Error() method (both fields): %s", err1.Error())
		t.Logf("String() method (both fields): %s", err1.String())

		// Test with only ExternalMsg set
		err2 := Error{
			Code:        500,
			ExternalMsg: "Internal server error",
		}

		t.Logf("Error() method (only ExternalMsg): %s", err2.Error())
		t.Logf("String() method (only ExternalMsg): %s", err2.String())

		// Test with only Err set
		err3 := Error{
			Code: 400,
			Err:  fmt.Errorf("validation failed: email is invalid"),
		}

		t.Logf("Error() method (only Err): %s", err3.Error())
		t.Logf("String() method (only Err): %s", err3.String())

		// Test with neither set
		err4 := Error{
			Code: 200,
		}

		t.Logf("Error() method (neither field): %s", err4.Error())
		t.Logf("String() method (neither field): %s", err4.String())
	})

	t.Run("Constructor Functions", func(t *testing.T) {
		// Test Errorf
		err1 := Errorf("User not found", "database query failed: user_id=%d not found", 123)
		t.Logf("Errorf result: %s", err1.Error())
		t.Logf("Errorf ExternalMsg: %s", err1.ExternalMsg)
		t.Logf("Errorf Err: %s", err1.Err.Error())

		// Test ErrorWithCodef
		err2 := ErrorWithCodef(404, "User not found", "database query failed: user_id=%d not found", 123)
		t.Logf("ErrorWithCodef result: %s", err2.Error())
		t.Logf("ErrorWithCodef Code: %d", err2.Code)
		t.Logf("ErrorWithCodef ExternalMsg: %s", err2.ExternalMsg)
		t.Logf("ErrorWithCodef Err: %s", err2.Err.Error())
	})

	t.Run("MarshalText and UnmarshalText", func(t *testing.T) {
		// Test MarshalText
		err1 := Error{
			Code:        404,
			ExternalMsg: "User not found",
			Err:         fmt.Errorf("database query failed"),
		}

		marshaled, _ := err1.MarshalText()
		t.Logf("MarshalText result: %s", string(marshaled))

		// Test UnmarshalText
		var err2 Error
		err2.UnmarshalText([]byte("Custom error message"))
		t.Logf("UnmarshalText result: %s", err2.Error())
		t.Logf("UnmarshalText ExternalMsg: %s", err2.ExternalMsg)
		t.Logf("UnmarshalText Err: %s", err2.Err.Error())
	})
}
