"""Main entrypoint for the convert script.

This script is used to convert Sigma rules to the target format per
each conversion object in the config.
---
usage: main.py [-h] [--config CONFIG] [--conversions-output-dir CONVERSIONS_OUTPUT_DIR]
               [--path-prefix PATH_PREFIX] [--render-traceback | --no-render-traceback]
               [--render-traceback | --no-render-traceback] [--pretty-print | --no-pretty-print]
               [--all-rules | --no-all-rules]
               [--changed-files CHANGED_FILES] [--deleted-files DELETED_FILES]

Sigma CLI Conversion

options:
  -h, --help            show this help message and exit
  --config CONFIG       Path to config YAML file (default: ./config.yaml)
  --path-prefix PATH_PREFIX
                        The path prefix to use for input files (default: .)
  --render-traceback, --no-render-traceback
                        Render traceback on error (default: False)
  --pretty-print, --no-pretty-print
                        Pretty print the converted files (default: False)
  --all-rules, --no-all-rules
                        Convert all rules (default: False)
  --changed-files CHANGED_FILES
                        List of changed files (default: )
  --deleted-files DELETED_FILES
                        List of deleted files (default: )
---
Notes:
  - The path prefix must be set using the PATH_PREFIX environment variable,
    or the GITHUB_WORKSPACE environment variable. PATH_PREFIX takes precedence.
  - The conversions output directory is set to "conversions" by default.
  - The config file must be a valid YAML file.
  - The config file must be present in the PATH_PREFIX directory.
  - The converted files will be saved in the PATH_PREFIX/conversions directory.
"""

from pathlib import Path

from convert import convert_rules
from settings import load_config, parse_args

if __name__ == "__main__":
    # Parse command line arguments and load config file.

    args = parse_args()
    config_file = Path(args.path_prefix) / Path(args.config)
    if not config_file.exists():
        raise FileNotFoundError(f"Config file not found: {config_file.resolve()}")
    config = load_config(str(config_file))

    # Convert Sigma rules to the target format per each conversion object in the config
    convert_rules(
        config=config,
        path_prefix=args.path_prefix,
        render_traceback=args.render_traceback,
        pretty_print=args.pretty_print,
        all_rules=args.all_rules,
        changed_files=args.changed_files,
        deleted_files=args.deleted_files,
    )
