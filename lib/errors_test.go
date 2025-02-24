package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractErrorCode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCode    uint32
		wantSuccess bool
	}{
		{
			name:        "Standard format with quotes",
			input:       "some text error code: '11' more text",
			wantCode:    11,
			wantSuccess: true,
		},
		{
			name:        "Alternative format with trailing colon",
			input:       "some text error code 11: more text",
			wantCode:    11,
			wantSuccess: true,
		},
		{
			name:        "Format without quotes",
			input:       "some text error code: 11 more text",
			wantCode:    11,
			wantSuccess: true,
		},
		{
			name:        "Format with quotes and trailing colon",
			input:       "some text error code: '11': more text",
			wantCode:    11,
			wantSuccess: true,
		},
		{
			name:        "Invalid format - no number",
			input:       "some text error code: abc more text",
			wantCode:    0,
			wantSuccess: false,
		},
		{
			name:        "No error code marker",
			input:       "some regular error message",
			wantCode:    0,
			wantSuccess: false,
		},
		{
			name:        "Empty message",
			input:       "",
			wantCode:    0,
			wantSuccess: false,
		},
		{
			name:        "Max uint32",
			input:       "error code: '4294967295'",
			wantCode:    4294967295,
			wantSuccess: true,
		},
		{
			name:        "Exceeds uint32",
			input:       "error code: '4294967296'",
			wantCode:    0,
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCode, gotSuccess := extractErrorCode(tt.input)
			assert.Equal(t, tt.wantSuccess, gotSuccess, "success result mismatch")
			if tt.wantSuccess {
				assert.Equal(t, tt.wantCode, gotCode, "extracted code mismatch")
			}
		})
	}
}

func TestParseGasFromOutOfGasError(t *testing.T) {
	tests := []struct {
		name           string
		errorMessage   string
		expectedWanted uint64
		expectedUsed   uint64
		expectError    bool
	}{
		{
			name:           "Valid out of gas error",
			errorMessage:   "gasWanted: 810, gasUsed: 1000: out of gas",
			expectedWanted: 810,
			expectedUsed:   1000,
			expectError:    false,
		},
		{
			name:           "Valid with extra spaces",
			errorMessage:   "gasWanted:    810,    gasUsed:   1000",
			expectedWanted: 810,
			expectedUsed:   1000,
			expectError:    false,
		},
		{ // nolint:exhaustruct
			name:         "Invalid format",
			errorMessage: "some other error",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Empty message",
			errorMessage: "",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Invalid numbers",
			errorMessage: "gasWanted: abc, gasUsed: def",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Partial message",
			errorMessage: "gasWanted: 810",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wanted, used, err := parseGasFromOutOfGasError(tt.errorMessage)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedWanted, wanted)
				assert.Equal(t, tt.expectedUsed, used)
			}
		})
	}
}

func TestParseSequenceFromAccountMismatchError(t *testing.T) {
	tests := []struct {
		name         string
		errorMessage string
		expectedExp  uint64
		expectedCurr uint64
		expectError  bool
	}{
		{
			name:         "Valid sequence mismatch",
			errorMessage: "account sequence mismatch, expected 5, got 3",
			expectedExp:  5,
			expectedCurr: 3,
			expectError:  false,
		},
		{
			name:         "Valid with extra spaces",
			errorMessage: "account sequence mismatch,    expected    10,     got    7",
			expectedExp:  10,
			expectedCurr: 7,
			expectError:  false,
		},
		{
			name:         "Valid with surrounding text",
			errorMessage: "error occurred: account sequence mismatch, expected 15, got 12: more details here",
			expectedExp:  15,
			expectedCurr: 12,
			expectError:  false,
		},
		{
			name:         "Large numbers",
			errorMessage: "account sequence mismatch, expected 999999, got 888888",
			expectedExp:  999999,
			expectedCurr: 888888,
			expectError:  false,
		},
		{ // nolint:exhaustruct
			name:         "Invalid format - wrong text",
			errorMessage: "some other error message",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Invalid format - missing numbers",
			errorMessage: "account sequence mismatch, expected , got ",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Invalid format - non-numeric values",
			errorMessage: "account sequence mismatch, expected abc, got def",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Empty message",
			errorMessage: "",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Partial message - only expected",
			errorMessage: "account sequence mismatch, expected 5",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Partial message - only got",
			errorMessage: "account sequence mismatch, got 3",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected, current, err := parseSequenceFromAccountMismatchError(tt.errorMessage)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedExp, expected)
				assert.Equal(t, tt.expectedCurr, current)
			}
		})
	}
}

func TestParseHTTPStatus(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedCode int
		expectedMsg  string
		expectError  bool
	}{
		{
			name:         "Standard format",
			input:        "Status: 404 Not Found",
			expectedCode: 404,
			expectedMsg:  "Not Found",
			expectError:  false,
		},
		{
			name:         "With extra text",
			input:        "Error occurred - Status: 500 Internal Server Error - more details",
			expectedCode: 500,
			expectedMsg:  "Internal Server Error",
			expectError:  false,
		},
		{
			name:         "Case insensitive",
			input:        "status: 429 Too Many Requests",
			expectedCode: 429,
			expectedMsg:  "Too Many Requests",
			expectError:  false,
		},
		{
			name:         "Extra spaces",
			input:        "Status:    503    Service Unavailable",
			expectedCode: 503,
			expectedMsg:  "Service Unavailable",
			expectError:  false,
		},
		{
			name:         "No message",
			input:        "Status: 200",
			expectedCode: 200,
			expectedMsg:  "",
			expectError:  false,
		},
		{ // nolint:exhaustruct
			name:        "Invalid format - no status code",
			input:       "Status: Not Found",
			expectError: true,
		},
		{ // nolint:exhaustruct
			name:        "Invalid format - wrong prefix",
			input:       "Error: 404 Not Found",
			expectError: true,
		},
		{ // nolint:exhaustruct
			name:        "Empty string",
			input:       "",
			expectError: true,
		},
		{ // nolint:exhaustruct
			name:        "Invalid status code",
			input:       "Status: abc Not Found",
			expectError: true,
		},
		{ // nolint:exhaustruct
			name:        "Negative status code",
			input:       "Status: -404 Not Found",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, msg, err := ParseHTTPStatus(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedCode, code, "status code mismatch")
				assert.Equal(t, tt.expectedMsg, msg, "status message mismatch")
			}
		})
	}
}

func TestParseInsufficientFeeError(t *testing.T) {
	tests := []struct {
		name         string
		errorMessage string
		denom        string
		expectedGot  uint64
		expectedReq  uint64
		expectError  bool
	}{
		{
			name:         "Valid insufficient fee error",
			errorMessage: "got: 163uallo required: 1625uallo: insufficient fee",
			denom:        "uallo",
			expectedGot:  163,
			expectedReq:  1625,
			expectError:  false,
		},
		{
			name:         "Complex error message with code and additional info",
			errorMessage: "error code 13: error checking fee: got: 200611uallo required: 2006110uallo, minGasPrice: 10.000000000000000000uallo, gas: 35888: insufficient fee",
			denom:        "uallo",
			expectedGot:  200611,
			expectedReq:  2006110,
			expectError:  false,
		},
		{
			name:         "Valid with extra spaces",
			errorMessage: "got:    163uallo    required:    1625uallo   : insufficient fee",
			denom:        "uallo",
			expectedGot:  163,
			expectedReq:  1625,
			expectError:  false,
		},
		{
			name:         "Valid with surrounding text",
			errorMessage: "error occurred: got: 163uallo required: 1625uallo: insufficient fee - more details",
			denom:        "uallo",
			expectedGot:  163,
			expectedReq:  1625,
			expectError:  false,
		},
		{
			name:         "Different denom",
			errorMessage: "got: 163atom required: 1625atom: insufficient fee",
			denom:        "atom",
			expectedGot:  163,
			expectedReq:  1625,
			expectError:  false,
		},
		{
			name:         "Large numbers",
			errorMessage: "got: 1000000uallo required: 9999999uallo: insufficient fee",
			denom:        "uallo",
			expectedGot:  1000000,
			expectedReq:  9999999,
			expectError:  false,
		},
		{ // nolint:exhaustruct
			name:         "Invalid format - wrong text",
			errorMessage: "some other error message",
			denom:        "uallo",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Invalid format - missing numbers",
			errorMessage: "got: uallo required: uallo: insufficient fee",
			denom:        "uallo",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Invalid format - non-numeric values",
			errorMessage: "got: abcuallo required: defuallo: insufficient fee",
			denom:        "uallo",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Empty message",
			errorMessage: "",
			denom:        "uallo",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Wrong denom",
			errorMessage: "got: 163uallo required: 1625uallo: insufficient fee",
			denom:        "atom",
			expectError:  true,
		},
		{
			name:         "Special regex characters in denom",
			errorMessage: "got: 163u.allo required: 1625u.allo: insufficient fee",
			denom:        "u.allo",
			expectedGot:  163,
			expectedReq:  1625,
			expectError:  false,
		},
		{ // nolint:exhaustruct
			name:         "Missing required part",
			errorMessage: "got: 163uallo: insufficient fee",
			denom:        "uallo",
			expectError:  true,
		},
		{ // nolint:exhaustruct
			name:         "Missing got part",
			errorMessage: "required: 1625uallo: insufficient fee",
			denom:        "uallo",
			expectError:  true,
		},
		{
			name:         "Zero values",
			errorMessage: "got: 0uallo required: 0uallo: insufficient fee",
			denom:        "uallo",
			expectedGot:  0,
			expectedReq:  0,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, required, err := parseInsufficientFeeError(tt.errorMessage, tt.denom)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedGot, got, "got fee mismatch")
				assert.Equal(t, tt.expectedReq, required, "required fee mismatch")
			}
		})
	}
}

func TestEstimateRequiredBaseGas(t *testing.T) {
	tests := []struct {
		name                  string
		gasWanted             uint64
		gasUsed               uint64
		baseGas               uint64
		excessCorrectionTimes int64
		expected              uint64
	}{
		{
			name:                  "Normal case",
			gasWanted:             100000,
			gasUsed:               150000,
			baseGas:               50000,
			excessCorrectionTimes: 1,
			expected:              120000, // (150000 - (100000 - 50000)) + 20000
		},
		{
			name:                  "Multiple excess corrections",
			gasWanted:             100000,
			gasUsed:               150000,
			baseGas:               50000,
			excessCorrectionTimes: 2,
			expected:              140000, // (150000 - (100000 - 50000)) + 40000
		},
		{
			name:                  "gasWanted <= baseGas",
			gasWanted:             40000,
			gasUsed:               150000,
			baseGas:               50000,
			excessCorrectionTimes: 1,
			expected:              150000, // returns gasUsed as it's larger
		},
		{
			name:                  "gasUsed <= dataGasEstimate (unusual case)",
			gasWanted:             150000,
			gasUsed:               60000,
			baseGas:               50000,
			excessCorrectionTimes: 1,
			expected:              100000, // dataGasEstimate (150000 - 50000) is larger
		},
		{
			name:                  "Zero excess correction",
			gasWanted:             100000,
			gasUsed:               150000,
			baseGas:               50000,
			excessCorrectionTimes: 0,
			expected:              100000, // 150000 - (100000 - 50000)
		},
		{
			name:                  "All zeros",
			gasWanted:             0,
			gasUsed:               0,
			baseGas:               0,
			excessCorrectionTimes: 1,
			expected:              0,
		},
		{
			name:                  "Very large numbers",
			gasWanted:             1000000,
			gasUsed:               1500000,
			baseGas:               500000,
			excessCorrectionTimes: 1,
			expected:              1020000, // (1500000 - (1000000 - 500000)) + 20000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateRequiredBaseGas(
				tt.gasWanted,
				tt.gasUsed,
				tt.baseGas,
				tt.excessCorrectionTimes,
			)
			assert.Equal(t, tt.expected, result,
				"For gasWanted=%d, gasUsed=%d, baseGas=%d, excessCorrectionTimes=%d, expected %d but got %d",
				tt.gasWanted, tt.gasUsed, tt.baseGas, tt.excessCorrectionTimes, tt.expected, result)
		})
	}
}
