# LazyDevOps

A tiny CLI that helps you quickly review Azure DevOps pull requests across repositories. It fetches recent PRs, shows their status (checks, votes, age, size), and lets you filter what you see.

## Features
- Lists open PRs from Azure DevOps
- Shows overall status of checks (Passed / In Progress / Failed / No checks / Unknown)
- Displays reviewers’ votes and PR metadata (author, source/target branch, created/updated time, size)
- Optional filtering by repository and number of PRs

## Installation
- From source (requires Go 1.21+):
  1. Clone this repo
  2. Build:
     - Windows PowerShell: `go build -o LazyDevOps.exe`
     - Other OS: `go build -o lazydevops`
- Binaries: If you use GoReleaser/GitHub Releases, download the artifact for your OS and architecture.

## Authentication
Create an Azure DevOps Personal Access Token with at least "Code (Read)" scope.

Set it as an environment variable before running the tool:
- Windows PowerShell: `$env:LAZY_DEV_OPS_PAT = "<your_pat_here>"`
- Linux/macOS: `export LAZY_DEV_OPS_PAT="<your_pat_here>"`

## Usage
```
Usage: lazydevops --org <org> --project <project> [--repo <repo>] [--top N]
Set LAZY_DEV_OPS_PAT environment variable with a Personal Access Token (Code: Read).
```

Flags:
- `--org`     Azure DevOps organization name (required)
- `--project` Azure DevOps project name (required)
- `--repo`    Repository name to filter (optional)
- `--top`     Max number of PRs to list (defaults to 100)

Examples:
- List PRs across all repos in a project:
  - `lazydevops --org myorg --project MyProject`
- List top 20 PRs for a specific repo:
  - `lazydevops --org myorg --project MyProject --repo my-repo --top 20`

Notes:
- The binary name may be `LazyDevOps.exe` on Windows and `lazydevops` on Unix-like systems.
- Output is a readable table; widths adapt to your terminal.

## Build from source
```
go build -o LazyDevOps.exe
```

## License
This project is released under the MIT License. See LICENSE for details.
