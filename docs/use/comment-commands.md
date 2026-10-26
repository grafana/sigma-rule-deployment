# Comment commands

You can run the convert and integrate stages on a pull request by commenting on it, for example, to convert every rule after you change a pipeline, or to re-run one configuration without query testing. This page describes the commands and who can use them.

## Before you begin

Your rule repository needs the [convert and integrate workflow](../set-up/workflows.md#convert-and-integrate) with its `issue_comment` trigger, merged to the default branch. GitHub only runs `issue_comment` workflows from the default branch.

## Commands

Post a comment on the pull request that starts with one of these commands:

```text
sigma convert <TARGET> [--no-test] [--debug] [--query-time-window <DURATION>]
sigma integrate <TARGET> [--no-test] [--debug] [--query-time-window <DURATION>]
```

For example, `sigma convert all` converts and integrates every rule, and `sigma integrate okta_audit --no-test` rebuilds the alert rules of the `okta_audit` configuration without testing their queries.

`convert` runs conversion and then integration. `integrate` runs integration only, from the conversion files already in the branch. Use it when you've only changed integration settings, such as labels or the rule group.

_TARGET_ is either `all` or a configuration name. `all` processes every rule in every configuration. A name selects configurations: an exact name selects that configuration, and a prefix selects every configuration whose name starts with it. For example, `ops` selects `ops_linux` and `ops_windows`. An exact match wins over a prefix match. Targets always select whole configurations, never single rules.

Keywords and flags aren't case-sensitive, but configuration names are. Only the first line of the comment is read, so you can explain the command on the lines below it.

## Flags

Both commands accept the following flags, in any order after the target:

| Flag                             | Effect                                                                                                                                                  |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--no-test`                      | Skip query testing for this run, whatever `test_queries` says.                                                                                          |
| `--debug`                        | Print tracebacks, indent the generated JSON, and include log lines in the query test results.                                                           |
| `--query-time-window <DURATION>` | Test queries over the last _DURATION_, for example, `30m`, `6h`, or `1d12h`, instead of the default range. A configuration's own `from` and `to` still take precedence. |

## Who can run commands

The workflow only acts on comments from users whose association with the repository is **Owner**, **Member**, or **Collaborator**, and on comments posted by automation, such as GitHub Apps. It ignores everyone else, and any comment that doesn't start with `sigma`.

## Result

The workflow reacts to a valid command with 👀 and runs it. When it finishes, it commits the generated files to the pull request branch and updates the result comments.

If the command is invalid, for example, an unknown flag or a target that matches no configuration, the workflow reacts with 😕 and replies with the usage and the list of valid configuration names. An invalid command never fails the workflow.

## Related pages

- [Set up the workflows](../set-up/workflows.md)
- [Configuration file](../configure/README.md)
