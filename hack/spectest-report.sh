#!/bin/bash

set -xe

# Constants
PROJECT_ROOT=$(pwd)
PRYSM_DIR="${PROJECT_ROOT%/hack}/testing/spectest"
EXCLUSION_LIST="$PRYSM_DIR/exclusions.txt"
REPORT_DIR="/tmp/spectest_report"
SPEC_REPO="git@github.com:ethereum/consensus-spec-tests.git"
SPEC_DIR="/tmp/consensus-spec"

# Create directory if it doesn't already exist
mkdir -p "$REPORT_DIR"

# Run spectests
go test ./testing/spectest/... -test.env SPEC_TEST_REPORT_OUTPUT_DIR="$REPORT_DIR" "$@"

# Ensure the SPEC_DIR exists and is a git repository
if [ -d "$SPEC_DIR/.git" ]; then
    echo "Repository already exists. Pulling latest changes."
    (cd "$SPEC_DIR" && git pull) || exit 1
else
    echo "Cloning the GitHub repository."
    git clone "$SPEC_REPO" "$SPEC_DIR" || exit 1
fi

# Finding all *_tests.txt files in REPORT_DIR and concatenating them into tests.txt
find "$REPORT_DIR" -type f -name '*_tests.txt' -exec cat {} + > "$PRYSM_DIR/tests.txt"

# Generating spec.txt
(cd "$SPEC_DIR" && find tests -maxdepth 4 -mindepth 4 -type d > "$PRYSM_DIR/spec.txt") || exit 1

# Comparing spec.txt with tests.txt and generating report.txt
while IFS= read -r line; do
   if grep -Fxq "$line" "$EXCLUSION_LIST"; then
      # If it's excluded and we have a test for it flag as an error
      if grep -q "$line" "$PRYSM_DIR/tests.txt"; then
          echo "Error: Excluded item found in tests.txt: $line"
          exit 1 # Exit with an error status
      else
          echo "Skipping excluded item: $line"
      fi
          continue
    fi
    if grep -q "$line" "$PRYSM_DIR/tests.txt"; then
        echo "found $line"
    else
        echo "missing $line"
    fi
done < "$PRYSM_DIR/spec.txt" > "$PRYSM_DIR/report.txt"

# Formatting report.txt
{
    echo "Prysm Spectest Report"
    echo ""
    echo "Tests Missing"
    grep '^missing' "$PRYSM_DIR/report.txt"
    echo ""
    echo "Tests Found"
    grep '^found' "$PRYSM_DIR/report.txt"
} > "$PRYSM_DIR/report_temp.txt" && mv "$PRYSM_DIR/report_temp.txt" "$PRYSM_DIR/report.txt"

# Check for the word "missing" in the report and exit with an error if present
if grep -q '^missing' "$PRYSM_DIR/report.txt"; then
    echo "Error: 'missing' tests found in report: $PRYSM_DIR/report.txt"
    exit 1
fi

# Clean up
rm -f "$PRYSM_DIR/tests.txt" "$PRYSM_DIR/spec.txt"
