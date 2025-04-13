#!/bin/bash
set -e

# Determine version bump type from commit messages
determine_bump_type() {
    if git log "$(git describe --tags --abbrev=0 2>/dev/null || echo 'v0.0.0')"..HEAD --pretty=format:%s | grep -q "^BREAKING CHANGE"; then
        echo "major"
    elif git log "$(git describe --tags --abbrev=0 2>/dev/null || echo 'v0.0.0')"..HEAD --pretty=format:%s | grep -q "^feat"; then
        echo "minor"
    else
        echo "patch"
    fi
}

# Get current version from Chart.yaml and bump it
CHART_FILE="helm/Chart.yaml"
CURRENT_VERSION=$(grep "version:" $CHART_FILE | head -1 | awk '{print $2}')
BUMP_TYPE=$(determine_bump_type)

if [[ "$BUMP_TYPE" == "major" ]]; then
    NEW_VERSION=$(echo $CURRENT_VERSION | awk -F. '{print $1+1".0.0"}')
elif [[ "$BUMP_TYPE" == "minor" ]]; then
    NEW_VERSION=$(echo $CURRENT_VERSION | awk -F. '{print $1"."$2+1".0"}')
else
    NEW_VERSION=$(echo $CURRENT_VERSION | awk -F. '{print $1"."$2"."$3+1}')
fi

echo "Bumping version from $CURRENT_VERSION to $NEW_VERSION"

# Update Chart.yaml
awk -v old="version: $CURRENT_VERSION" -v new="version: $NEW_VERSION" '{gsub(old, new); print}' $CHART_FILE > tmp && mv tmp $CHART_FILE

# Update values.yaml image tag
VALUES_FILE="helm/values.yaml"
awk -v old="tag: v$CURRENT_VERSION" -v new="tag: v$NEW_VERSION" '{gsub(old, new); print}' $VALUES_FILE > tmp && mv tmp $VALUES_FILE

# Update CHANGELOG.md
DATE=$(date +%Y-%m-%d)
awk -v date="$DATE" -v new_ver="$NEW_VERSION" '/## \[Unreleased\]/{print; print ""; print "## [" new_ver "] - " date; next}1' CHANGELOG.md > tmp && mv tmp CHANGELOG.md

echo "NEW_VERSION=$NEW_VERSION"
echo "new_version=${NEW_VERSION}" >> $GITHUB_OUTPUT

echo "Version bumped to $NEW_VERSION"