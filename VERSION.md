# ProofOfAccess Versioning

## How to Release

### Automatic Release (Recommended)

1. Go to the [Actions tab](../../actions) on GitHub
2. Click on "Build and Release" workflow
3. Click "Run workflow"
4. Enter the version number (e.g., 0.2.0)
5. Click "Run workflow"

The workflow will:
- Build binaries for all platforms
- Create a GitHub release with the binaries
- Generate release notes automatically

### Manual Release

Add `[release]` to your commit message when pushing to main:
```bash
git commit -m "Fix: Storage node connection [release]"
git push origin main
```

## Version Information

The binary supports the `-version` flag:
```bash
./proofofaccess -version
```

This will output:
- Version number
- Build time
- Git commit ID