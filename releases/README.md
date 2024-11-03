# Orra Releases

This directory manages release artifacts and changelogs for all Orra components. It follows a structured approach to track changes and automate the release process through GitHub Actions.

## Directory Structure

```
releases/
├── README.md (this file)
├── cli/
│   ├── CHANGELOG.md
│   └── versions/
│       ├── 0.1.0.md
│       └── 0.1.1.md
├── controlplane/
│   ├── CHANGELOG.md
│   └── versions/
│       └── 0.1.0.md
└── sdks/
    ├── CHANGELOG.md
    └── versions/
        └── 0.1.0.md
```

Each component has its own directory containing:
- A rolling `CHANGELOG.md` that aggregates all versions
- A `versions` directory with individual version changelog files

## Release Types

Orra supports both individual component releases and full releases:

### Individual Component Releases
- CLI: Use tag format `cli-v*.*.*` (e.g., `cli-v0.1.0`)
- Control Plane: Use tag format `cp-v*.*.*` (e.g., `cp-v0.1.0`)
- SDK: Use tag format `sdk-v*.*.*` (e.g., `sdk-v0.1.0`)

### Full Releases
- Use tag format `v*.*.*` (e.g., `v0.1.0`)
- Requires changelog files for all updated components

## Creating a Release

1. **Create Changelog File**

   Create a new markdown file in the appropriate component's versions directory:

   ```bash
   # For CLI version 0.1.0
   vim releases/cli/versions/0.1.0.md
   ```

   Example changelog format:
   ```markdown
   ## CLI v0.1.0

   ### Features
   - Added support for webhook management
   - Added new `projects` command for project management

   ### Bug Fixes
   - Fixed authentication token refresh
   - Improved error messages for API failures

   ### Breaking Changes
   - Renamed `orra service` to `orra register service`

   ### Documentation
   - Added examples for webhook configuration
   - Updated installation instructions
   ```

2. **Commit Changes**
   ```bash
   git add releases/cli/versions/0.1.0.md
   git commit -m "chore: add CLI 0.1.0 changelog"
   ```

3. **Create and Push Tag**
   ```bash
   # For CLI only
   git tag cli-v0.1.0
   git push origin cli-v0.1.0

   # For full release
   git tag v0.1.0
   git push origin v0.1.0
   ```

4. **Review and Publish**
    - GitHub Actions will create a draft release
    - Review the generated release notes
    - Verify binary artifacts and checksums (for CLI releases)
    - Publish the release when ready

## Release Artifacts

### CLI Releases
- Binary distributions for:
    - Linux (amd64)
    - macOS (amd64, arm64)
    - Windows (amd64)
- SHA256 checksums for each binary

### Control Plane Releases
- Docker Compose configuration updates
- Documentation for deployment changes

### SDK Releases
- Updated NPM package versions
- API documentation updates

## Release Workflow

The GitHub Actions workflow automatically:

1. Detects the type of release based on the tag
2. Verifies the existence of appropriate changelog files
3. Builds and packages required artifacts
4. Generates comprehensive release notes
5. Creates a draft release for review

## Guidelines

### Changelog Best Practices

1. **Clear Sections**
    - Features
    - Bug Fixes
    - Breaking Changes
    - Documentation
    - Dependencies
    - Security Updates

2. **Entry Format**
    - Use present tense
    - Start with a verb
    - Be specific but concise
    - Reference issue/PR numbers where applicable

3. **Breaking Changes**
    - Always highlight breaking changes prominently
    - Provide migration instructions or links

### Version Numbers

Follow [Semantic Versioning](https://semver.org/):
- MAJOR version for incompatible API changes
- MINOR version for backwards-compatible functionality
- PATCH version for backwards-compatible bug fixes
- Additional labels for pre-release (e.g., `-alpha.1`, `-beta.1`)

## Troubleshooting

### Missing Changelog
If the release workflow fails with a missing changelog error:
1. Verify the changelog file exists in the correct location
2. Ensure the version number matches the tag
3. Check file permissions and git tracking

### Failed Builds
For CLI build failures:
1. Check the GitHub Actions logs
2. Verify Go version compatibility
3. Ensure all dependencies are available

### Recommended Commands

```bash
# List all tags
git tag

# Delete a local tag (if needed)
git tag -d cli-v0.1.0

# Delete a remote tag (if needed)
git push --delete origin cli-v0.1.0

# View changelog file
cat releases/cli/versions/0.1.0.md
```

## Contributing

1. Follow the changelog format specified above
2. Use conventional commit messages
3. Always create changelog files before tagging
4. Test release notes locally when possible

## Support

For questions about the release process:
1. Open an issue with the label `release-process`
2. Contact the maintainers
3. Check existing documentation and past releases
