# Test Coverage Guide

This document describes the test coverage tools and analysis available for loqa-hub.

## Coverage Tools Setup

### Prerequisites

All coverage tools are automatically installed when you run:

```bash
make install-tools
```

This installs:
- `golang.org/x/tools/cmd/cover@latest` - Go coverage analysis tool
- `golangci-lint` - Comprehensive linting (includes coverage-related checks)

### Manual Installation

If you need to install coverage tools manually:

```bash
go install golang.org/x/tools/cmd/cover@latest
```

## Coverage Analysis Commands

### Quick Coverage Check

```bash
# Simple coverage with HTML output
make test-coverage
```

This runs tests with basic coverage and opens an HTML report.

### Comprehensive Coverage Analysis

```bash
# Detailed coverage analysis with enhanced reporting
make coverage
```

This runs the comprehensive coverage script that:
- Tests each package individually
- Generates detailed coverage reports
- Creates combined coverage statistics
- Provides color-coded output with coverage thresholds

### Coverage with HTML Report

```bash
# Generate coverage and open HTML report in browser
make coverage-html
```

## Coverage Thresholds

The coverage analysis uses the following thresholds:

- **🟢 Good**: ≥80% coverage
- **🟡 Acceptable**: 60-79% coverage
- **🔴 Needs Improvement**: <60% coverage

## Current Coverage Status

Based on latest analysis:

| Package | Coverage | Status |
|---------|----------|--------|
| `arbitration` | 93.5% | 🟢 Excellent |
| `config` | 88.2% | 🟢 Good |
| `intent` | 90.9% | 🟢 Excellent |
| `tiers` | 95.9% | 🟢 Excellent |
| `transport` | 62.6% | 🟡 Acceptable |
| `server` | 22.9% | 🔴 Needs Improvement |

**Overall Coverage**: 71.4% (Acceptable)

## Coverage Files

Coverage reports are saved in the `coverage/` directory:

- `coverage/combined.out` - Combined coverage data
- `coverage/coverage.html` - HTML coverage report
- `coverage/{package}.out` - Individual package coverage files

## Improving Coverage

### Focus Areas

1. **Server Package (22.9%)**: Add integration tests for HTTP handlers and component initialization
2. **Transport Package (62.6%)**: Add tests for edge cases and error handling in streaming transport

### Adding Tests

When adding new tests:

1. Follow existing test patterns in each package
2. Test both success and error cases
3. Use table-driven tests for multiple scenarios
4. Mock external dependencies appropriately

### Coverage-Driven Development

1. Run coverage analysis before and after changes
2. Aim for >80% coverage on new code
3. Use coverage reports to identify untested code paths
4. Focus on critical business logic first

## Integration with CI/CD

The coverage tools are integrated into the quality check process:

```bash
# Run all quality checks including tests
make quality-check

# Pre-commit checks (faster subset)
make pre-commit
```

## Troubleshooting

### "no such tool covdata" Error

This error appears for packages without test files and is normal. It doesn't affect coverage analysis for packages that do have tests.

### Coverage Not Generating

1. Ensure you're in the project root directory
2. Verify test files exist with `*_test.go` naming
3. Check that tests compile and run: `go test ./...`
4. Make sure coverage tools are installed: `make install-tools`

### HTML Report Not Opening

The HTML report is always generated at `coverage/coverage.html` even if the browser doesn't open automatically. You can open it manually in any web browser.