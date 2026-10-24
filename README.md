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

a tracked file can have variants for different machines. say which machines
a file is for when you add it:

    dotmgr add ~/.zshrc --os                # only on this os (darwin, ...)
    dotmgr add ~/.zshrc --os=linux          # the version for linux machines
    dotmgr add ~/.ssh/config --host         # only on this host
    dotmgr add ~/.gitconfig --class=work    # machines of class "work"

only the variant that fits the machine best is put on disk, under the
file's real name; the others stay in the repository. a plainly added file is
the fallback for machines that no variant fits. `--os` and friends without a
value mean this machine; values need `=`. several conditions must all match.

- `--os[=…]`: linux, darwin (or macos), windows, freebsd, …; `unix` is
  every system but windows
- `--host[=…]`: the host name without domain
- `--class[=…]`: set a machine's class with `dotmgr config local.class work`;
  changing it switches files to the variants that fit now
- `--arch[=…]`: amd64, arm64, …

when several variants fit, the most specific wins: host > os > unix >
class > arch > plain file. adding a variant for this machine to a file that
was tracked plainly limits the file to such machines; a variant for other
machines (say `--os=linux` on a mac) is stored next to it and leaves the
file on disk alone. once a file has variants, `dotmgr add ~/.zshrc` updates
the one in use, and `dotmgr rm ~/.zshrc` untracks it (`dotmgr rm ~/.zshrc
--os=linux` names another). `dotmgr list` shows all variants.

in the repository a variant is stored as `<file>##<condition>,…`, the way
yadm names them (`.zshrc##os.darwin`), so the repository stays readable
with plain git. dotmgr 0.3 kept those `##` files on disk and linked to them;
the first command you run after upgrading moves them back.

think twice before tracking secrets such as private ssh keys: `dotmgr push`
sends them to the remote.

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
