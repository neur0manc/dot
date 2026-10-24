# dot

config management akin to yadm. no-dependency, single, static binary
for linux, windows and macos.

## installation

linux / macos:

    curl -fsSL https://raw.githubusercontent.com/neur0manc/dot/main/install.sh | sh

windows (powershell):

    irm https://raw.githubusercontent.com/neur0manc/dot/main/install.ps1 | iex

the installers download the archive of the latest release for your platform,
verify it against the release's `SHA256SUMS`, and put it into
`~/.local/bin` (`%LOCALAPPDATA%\Programs\dot` on windows, which is also
added to your user PATH). pick a version with `DOT_VERSION=v0.4.0` and a
directory with `DOT_INSTALL_DIR`. you can also download the archives from
the [releases page](https://github.com/neur0manc/dot/releases), or build
from source with `go install github.com/neur0manc/dot/cmd/dot@latest`.

## releasing

    scripts/release.sh v0.4.0
    git push origin HEAD v0.4.0

`release.sh` runs the tests and creates the tag. pushing it starts the
release workflow on github actions, which cross-compiles all platforms with
`scripts/build.sh` and publishes the archives and `SHA256SUMS` as a github
release. every push and pull request runs the tests on linux, macos and
windows.

## usage

dot works like yadm: your home directory is the working tree, the files
stay where they are, and only files you explicitly add are tracked. The git
directory lives outside of it (`~/.local/share/dot/repo.git`, or
`%LOCALAPPDATA%\dot\repo.git` on windows). the project used to be called
dotmgr: a repository in `~/.local/share/dotmgr` is still found, and the
environment variables are now called `DOT_…` instead of `DOTMGR_…`.

    dot init                      # or: dot clone <url>
    dot add ~/.vimrc ~/.config/foo
    dot status                    # staged / modified tracked files
    dot ls .                      # tracked files here and below
    dot diff
    dot commit -m "add vim config"
    dot remote add origin <url>
    dot push

on another machine `dot clone <url>` checks the files out into `$HOME`.
files that already exist with different content are never overwritten
unless you pass `--force` (also for `pull` and `checkout`).

### per-os and per-host files

a tracked file can have variants for different machines. say which machines
a file is for when you add it:

    dot add ~/.zshrc --os                # only on this os (darwin, ...)
    dot add ~/.zshrc --os=linux          # the version for linux machines
    dot add ~/.ssh/config --host         # only on this host
    dot add ~/.gitconfig --class=work    # machines of class "work"

only the variant that fits the machine best is put on disk, under the
file's real name; the others stay in the repository. a plainly added file is
the fallback for machines that no variant fits. `--os` and friends without a
value mean this machine; values need `=`. several conditions must all match.

- `--os[=…]`: linux, darwin (or macos), windows, freebsd, …; `unix` is
  every system but windows
- `--host[=…]`: the host name without domain
- `--class[=…]`: set a machine's class with `dot config local.class work`;
  changing it switches files to the variants that fit now
- `--arch[=…]`: amd64, arm64, …

when several variants fit, the most specific wins: host > os > unix >
class > arch > plain file. adding a variant for this machine to a file that
was tracked plainly limits the file to such machines; a variant for other
machines (say `--os=linux` on a mac) is stored next to it and leaves the
file on disk alone. once a file has variants, `dot add ~/.zshrc` updates
the one in use, and `dot rm ~/.zshrc` untracks it (`dot rm ~/.zshrc
--os=linux` names another). `dot list` shows all variants.

in the repository a variant is stored as `<file>##<condition>,…`, the way
yadm names them (`.zshrc##os.darwin`), so the repository stays readable
with plain git. dotmgr 0.3 kept those `##` files on disk and linked to them;
the first command you run after upgrading moves them back.

think twice before tracking secrets such as private ssh keys: `dot push`
sends them to the remote.

### a readme for the repository

the `README.md` at the root of the repository, which GitHub shows on the
repository's page, lives at `~/.local/dot/README.md` on disk instead of in
your home directory. edit it there and track it like any other file:

    dot add ~/.local/dot/README.md
    dot commit -m "describe my dotfiles"

### private files

files under `~/.ssh` and `~/.gnupg` are written readable only by you (0600,
or 0700 if executable), and dot creates those directories as 0700. checkout,
pull and clone also tighten such files that are readable by others, as dot
0.4 left them. elsewhere, a file dot overwrites keeps the permissions it has.

ssh remotes authenticate like `ssh` does: keys of the running ssh-agent
first, then `IdentityFile` entries from `~/.ssh/config` and the default
key files in `~/.ssh`. encrypted key files ask for their passphrase on the
terminal (or read `DOT_SSH_PASSPHRASE`); host keys are checked against
`~/.ssh/known_hosts`. https remotes use `DOT_HTTPS_USERNAME` and
`DOT_HTTPS_PASSWORD`.

`dot help` lists all commands; `ls`, `s`, `d` and `p` are short for `list`,
`status`, `diff` and `push`. Without `-m`, `dot commit` opens an editor for
the message, chosen as git chooses it: `GIT_EDITOR`, then `core.editor` from
the repository's and your global git configuration, then `VISUAL`, then
`EDITOR`. `DOT_HOME` and `DOT_REPO` override
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
