#!/bin/bash

# Copyright (c) 2025 HYPR. PTE. LTD.
#
# Business Source License 1.1
# See LICENSE file in the project root for details.


# Script to add BSL license headers to all source code files
# This script processes .go and .sh files, but excludes documentation files

set -e

# Define the license header for Go files
GO_HEADER='// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.'

# Define the license header for shell scripts
SH_HEADER='# Copyright (c) 2025 HYPR. PTE. LTD.
#
# Business Source License 1.1
# See LICENSE file in the project root for details.'

# Function to add header to Go files
add_go_header() {
    local file="$1"

    # Check if file already has a license header
    if head -n 1 "$file" | grep -q "// Copyright"; then
        echo "Skipping $file (already has license header)"
        return
    fi

    echo "Adding license header to $file"

    # Create temporary file with header + original content
    local temp_file=$(mktemp)
    echo "$GO_HEADER" > "$temp_file"
    echo "" >> "$temp_file"
    cat "$file" >> "$temp_file"

    # Replace original file
    mv "$temp_file" "$file"
}

# Function to add header to shell script files
add_sh_header() {
    local file="$1"

    # Check if file already has a license header
    if head -n 1 "$file" | grep -q "# Copyright"; then
        echo "Skipping $file (already has license header)"
        return
    fi

    echo "Adding license header to $file"

    # Handle shebang
    local shebang=""
    if head -n 1 "$file" | grep -q "#!"; then
        shebang=$(head -n 1 "$file")
        tail -n +2 "$file" > "${file}.tmp"
        mv "${file}.tmp" "$file"
    fi

    # Create temporary file with header + original content
    local temp_file=$(mktemp)

    # Add shebang if it existed
    if [[ -n "$shebang" ]]; then
        echo "$shebang" > "$temp_file"
        echo "" >> "$temp_file"
    fi

    # Add license header
    echo "$SH_HEADER" >> "$temp_file"
    echo "" >> "$temp_file"

    # Add original content
    cat "$file" >> "$temp_file"

    # Replace original file
    mv "$temp_file" "$file"
}

# Find and process Go files
echo "Processing Go files..."
find . -name "*.go" -not -path "./docs/*" -not -path "./vendor/*" | while read -r file; do
    add_go_header "$file"
done

# Find and process shell script files
echo "Processing shell script files..."
find . -name "*.sh" -not -path "./docs/*" -not -path "./vendor/*" | while read -r file; do
    add_sh_header "$file"
done

echo "License headers added successfully!"