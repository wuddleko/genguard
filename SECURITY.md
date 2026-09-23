# Security

## Reporting a vulnerability

Report a vulnerability in genguard privately: [Open a GitHub security advisory](https://github.com/wuddleko/genguard/security/advisories/new).

Include the version (`genguard version`), the config you ran, and what happened. Keep the report off the public issue tracker until a fix is out.

A report is a case where genguard did something the config did not ask for. Running `command`, deleting the paths listed under `outputs` when `clean` is set, and printing a diff of those paths are what the tool is for.

## What a run is allowed to do

`command` runs in a shell as you, from the directory that holds the config. A `genguard.yaml` is the same kind of file as a workflow or a Makefile: read it before you run it. `genguard check --all` runs every config it finds under the repo.

`clean` refuses a path that would delete `.git`, the config file, a symlink, or anything outside the config directory. That stops a bad `outputs` entry from wiping the checkout. The shell command still has your permissions, and it can read, write, and delete whatever you can.

A failed command includes its output in the error. Drift includes a git diff. Whatever the generator prints, or writes into those files, shows up in the log.

## Releases

Download archives from [GitHub Releases](https://github.com/wuddleko/genguard/releases) and check them against the `checksums.txt` published with that release. [install.sh](install.sh) does that check for the release it downloads. Pin an `install.sh` URL to a release tag that contains the script. Fixes ship on the latest release.
