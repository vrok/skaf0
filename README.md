# Skaf0

## Introduction

Skaf0 is a wrapper for Skaffold that gives you more control over which artifacts are rebuilt and when.

This is particularly useful for projects with many artifacts that depend on the same source files. A change to one of those files may cause many artifacts to be rebuilt and redeployed, which may not be desirable.

In dev mode, Skaf0 does not automatically rebuild artifacts based on file system changes. Instead, you trigger the rebuilds manually, allowing you to choose which specific artifacts should be rebuilt and deployed.

## Installation

You can install it with the following command (unfortunately, `go install ...` doesn't work, see [golang/go#44840](https://github.com/golang/go/issues/44840)):

```
cd $(mktemp -d) && git clone --depth 1 https://github.com/vrok/skaf0 \
 . && go build -o skaf0 && sudo mv skaf0 /usr/local/bin/
 ```

## Usage

In a terminal, instead of running `skaffold dev`, run:

```sh
skaf0 dev
```

You can add the same extra arguments that you would use with `skaffold dev` (e.g. `--port-forward` and others).

Once everything is up and running, run the following command in another terminal to get a list of all detected artifacts:

```sh
skaf0 ctrl list
```

You can now trigger one of them to be rebuilt:

```sh
skaf0 ctrl rebuild frontend
```

For example:

```
$ skaf0 ctrl list
backend
frontend

$ skaf0 ctrl rebuild frontend
Rebuild triggered for artifacts: frontend
```

You can also trigger multiple artifacts to be rebuilt at once:

```sh
skaf0 ctrl rebuild frontend backend
```

And finally, you can rebuild them using wildcard expressions:

```sh
# Rebuild only artifacts with names starting with 'backend-'
skaf0 ctrl rebuild 'backend-*'

# Rebuild all artifacts
skaf0 ctrl rebuild '*'
```

If you don't provide any artifact names or patters, it will display a simple selector:

```
$ skaf0 ctrl rebuild
Use the arrow keys to navigate: ↓ ↑ → ←
? Select artifact to rebuild:
  ▸ frontend
    backend
```

