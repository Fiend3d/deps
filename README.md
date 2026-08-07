# deps

A lightweight TUI dependency walker - inspect imports, exports, and ordinal mappings of Windows PE files (EXE/DLL).

Built into `mc`: highlight a file and press **F2**.

Or use it standalone:
```
Usage: deps [flags] <filepath>
  -v    print version
  -i    print imports
  -e    print exports
  -r    print recursive dependencies
  -u    print only unresolved dependencies (recursive)
  -p    print without color
  -d    hide delay-loaded dependencies
```

Color is dropped automatically when output is redirected, and when `NO_COLOR` is set.

## Checking for missing dependencies

`-u` walks the whole graph and reports only what failed to resolve, naming the
modules that import it:

```
$ deps -u C:\Windows\System32\notepad.exe
C:\Windows\System32\notepad.exe MSVC 14.38.33145
AzureAttestManager.dll (delay)  <- DMCmnUtils.dll
HvsiFileTrust.dll (delay)       <- SHELL32.dll

2 unresolved
```

It exits **1 only when a missing module is needed at load time** — that is what
stops an image from starting. A missing delay-load is reported but exits 0,
since it fails only if that code path is called, and a healthy Windows install
always has a few. API set contracts are never counted: they have no file on disk
by design and the loader resolves them through its schema.

`-u` stands alone and ignores `-i`/`-e`/`-r`; `-p` and `-d` still apply.

The same report is on `u` in the TUI, with the importers on the right of each
row. Unlike the tree, which resolves a node at a time as you open it, this walks
the whole graph at once — the first press can take a moment on a large image,
and the result is kept for as long as you stay on that file.

## Hiding delay-loaded modules

A delay-loaded import binds on the first call into it rather than at process
start, so it is the loader's weak edge. `-d` drops those modules from every
listing, along with the subtrees reachable only through them:

```
$ deps -u -d C:\Windows\System32\notepad.exe
C:\Windows\System32\notepad.exe MSVC 14.38.33145
No unresolved dependencies
```

With `-u` it leaves the load-time findings and the exit code untouched, which
makes `-u -d` a clean "will this image start" check. It is off by default: the
listing should agree with the image's import table unless you ask otherwise.

In the TUI the same filter is on `d`, and the header shows `-delay` while rows
are being withheld.

## Keys

| Key | Action |
| --- | --- |
| `Tab` | switch between imports and exports |
| `r` | recursive dependency tree |
| `u` | unresolved dependencies (recursive) |
| `Space` | expand / collapse |
| `Enter` | open the selected module |
| `h` / `left` | back (in the tree: collapse, or jump to the parent) |
| `d` | show / hide delay-loaded modules |
| `c` / `p` / `f` / `a` | copy name / path / functions / all |
| `q` | quit |

Modules are marked `(delay)` when delay-loaded and `(api-set)` when they are API
set contracts, which the loader resolves through its schema rather than from a
file on disk. In the tree, `(cycle)` marks a module already open above itself;
with `-r`, `(seen)` marks one already expanded elsewhere.
