# perfscan as a golangci-lint plugin

This module exposes perfscan's checks as a golangci-lint **module plugin**, so you
can run them inside your existing golangci-lint pipeline (one binary, unified output).

## Build a custom golangci-lint with perfscan

Create `.custom-gcl.yml`:

```yaml
version: v2.12.2                                   # your golangci-lint version
name: perfscan-gcl
destination: .
plugins:
  - module: github.com/jxsl13/perfscan/perfscan/plugin
    version: v0.34.0                               # a published perfscan release
```

Then `golangci-lint custom` builds `./perfscan-gcl`.

## Enable it

In `.golangci.yml`:

```yaml
version: "2"
linters:
  enable:
    - perfscan
  settings:
    custom:
      perfscan:
        type: module
        description: "Go performance linter"
        settings:
          maxLevel: 3          # 1=idiomatic, 2=structured, 3=aggressive
          # vocabulary: {...}  # optional; same shape as perfscan.yaml for domain checks
```

`./perfscan-gcl run ./...` then reports perfscan findings, e.g.:

```
a.go:5:20: PS3104: sort.Ints is the legacy spelling of slices.Sort … (perfscan)
```

The plugin registers every check from perfscan's registry (via `checks.All()`), so
it always exposes the full catalog of the pinned release.

## Note for maintainers

The `version:` above (and `plugin/go.mod`'s `require github.com/jxsl13/perfscan/perfscan`)
must point at a perfscan release that still EXISTS. Because the release policy keeps
only the last 3 perfscan releases, bump this pin on release so it stays within the
kept set — otherwise `golangci-lint custom` fails with `unknown revision`. In-repo
builds are unaffected (`plugin/go.mod` has `replace … => ../`), which is why CI's
`go build ./...` can stay green even when the pin has aged out.
