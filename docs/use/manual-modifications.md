# Preserve manual modifications

The convert and integrate stages regenerate the conversion files and alert rule files on every run, which normally overwrites any change you make to them by hand. If you need to edit a generated file, for example, to tune a query the converter can't express, mark it as manual so the Actions leave it alone.

## Mark a file as manual

Add a `manual` flag to the file:

- In a conversion file, add `"manual": true` at the top level.
- In an alert rule file, add `"manual": "true"` to the rule's `annotations`. It's an annotation so the alert rule stays valid.

The Actions never overwrite or delete a file with the flag set. That includes when you remove its Sigma rule, and when the file would otherwise be cleaned up as orphaned.

## Automatic detection

You usually don't need to add the flag yourself. When you edit a generated file and push to the pull request, the next run compares the file with the last automation commit, sees that a person changed it, and adds the flag in its "Sigma Rules Deployment" commit. Your edit is kept from then on, and the run logs which file it kept.

## Hand a file back to automation

To let the Actions manage a file again, set the flag to false: `"manual": false` in a conversion file, or `"manual": "false"` in an alert rule file's annotations. The Actions regenerate the file the next time its Sigma rule is converted, for example, when you change the rule or comment `sigma convert <CONFIGURATION_NAME>`.

> [!IMPORTANT]
> Don't delete the flag. Automatic detection can't tell a deleted flag from any other human edit, so the next run adds the flag back.

## Related pages

- [How Sigma Rule Deployment works](../concepts/how-it-works.md#generated-files)
