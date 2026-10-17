# dotmgr

config management akin to yadm. no-dependency, single, static binary
for linux, windows and macos.

## installation

linux / macos:

    curl -fsSL https://git.sr.ht/~stephan/dotmgr/blob/main/install.sh | sh

windows (powershell):

    irm https://git.sr.ht/~stephan/dotmgr/blob/main/install.ps1 | iex

the installers download the archive of the latest release for your platform,
verify it against the release's `SHA256SUMS`, and put it into
`~/.local/bin` (`%LOCALAPPDATA%\Programs\dotmgr` on windows, which is also
added to your user PATH). pick a version with `DOTMGR_VERSION=v0.2.0` and a
directory with `DOTMGR_INSTALL_DIR`. you can also download the archives and
`SHA256SUMS` by hand from
`https://git.sr.ht/~stephan/dotmgr/refs/download/<tag>/`, or build from
source with `go install git.sr.ht/~stephan/dotmgr/cmd/dotmgr@latest`.

## releasing

    scripts/release.sh v0.2.0
    git push origin HEAD v0.2.0

`release.sh` runs the tests, records the tag in `RELEASE` (which the
installers read to find the latest release) and creates the tag. the build
on builds.sr.ht (`.build.yml`) then cross-compiles all platforms and, for
tagged commits, also attaches the archives to the tag on git.sr.ht, where
they do not expire like build artifacts do. the upload needs a git.sr.ht
token stored as a builds.sr.ht secret, see the comment in `.build.yml`.

## usage

dotmgr works like yadm: your home directory is the working tree, the files
stay where they are, and only files you explicitly add are tracked. The git
directory lives outside of it (`~/.local/share/dotmgr/repo.git`, or
`%LOCALAPPDATA%\dotmgr\repo.git` on windows).

    dotmgr init                      # or: dotmgr clone <url>
    dotmgr add ~/.vimrc ~/.config/foo
    dotmgr status                    # staged / modified tracked files
    dotmgr diff
    dotmgr commit -m "add vim config"
    dotmgr remote add origin <url>
    dotmgr push

on another machine `dotmgr clone <url>` checks the files out into `$HOME`.
files that already exist with different content are never overwritten
unless you pass `--force` (also for `pull` and `checkout`).

### per-os and per-host files

like yadm, a tracked file can have alternates for different systems: name
them `<file>##<condition>[,<condition>...]`, for example

    ~/.gitconfig##default
    ~/.gitconfig##os.Darwin
    ~/.gitconfig##hostname.work,os.Linux
    ~/.gitconfig##class.laptop

to turn an existing file into an alternate, let dotmgr do the renaming:

    dotmgr add --alt hostname.work,os.Linux ~/.gitconfig

this renames `~/.gitconfig` to `~/.gitconfig##hostname.work,os.Linux`, tracks
it and links the old name back to it. if the alternate does not apply to
the machine you are on (say `--alt os.Windows` on a mac), the file is copied
instead, so nothing you use disappears. permissions are kept. think twice
before tracking secrets such as private ssh keys: `dotmgr push` sends
them to the remote.

`dotmgr alt` makes `~/.gitconfig` a symbolic link to the best match (a copy
on windows) and it runs automatically after `clone`, `pull` and `checkout`.
conditions: `default`, `os.` (linux, darwin or macos, windows), `arch.`,
`class.` (set with `dotmgr config local.class laptop`), `hostname.` (without
domain) and `user.`. matching ignores case; all conditions of an alternate
must match, and the one with the heaviest conditions wins (user > hostname >
os > class > arch > default). add the `##` files, not the generated one;
dotmgr never replaces a file it did not create unless you pass `--force`.
templates (`##template`) are not supported.

ssh remotes authenticate like `ssh` does: keys of the running ssh-agent
first, then `IdentityFile` entries from `~/.ssh/config` and the default
key files in `~/.ssh`. encrypted key files ask for their passphrase on the
terminal (or read `DOTMGR_SSH_PASSPHRASE`); host keys are checked against
`~/.ssh/known_hosts`. https remotes use `DOTMGR_HTTPS_USERNAME` and
`DOTMGR_HTTPS_PASSWORD`.

`dotmgr help` lists all commands. `DOTMGR_HOME` and `DOTMGR_REPO` override
the working tree and git directory locations.

## banter

### q: why make this when yadm is great?

yadm really is great, used it for years. but

- it's not natively available on windows
- it requires git
- (and python)

and i personally am a 'single-static-binary-no-deps'-enjoyer which
is a thing.

### q: but go?

go really is great indeed

- native git library available
- cross-compiling on alpine linux for all platforms
- no giant .net installation for compiling on windows
- honestly i also just like the syntax
- 50 mb binary size for a simple hello world seem to be solved as well
  (although that was never that big of a deal but it's nice)

but most important: it has a good, cute mascot

### disclaimer

i played product manager in this project, the role of eager junior
software engineer was played by a certain farting cat, named 5.6 sol.
