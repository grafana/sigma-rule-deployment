"""Settings for the conversion action."""

import argparse
import os

from dynaconf import Dynaconf


def parse_args() -> argparse.Namespace:
    """
    Parse command line arguments to get config file.

    Returns:
        argparse.Namespace: Parsed command line arguments containing:
            - config: Path to config YAML file (Path)
            - path_prefix: Path prefix for input files (Path)
            - render_traceback: Whether to render traceback on error (boolean)
            - pretty_print: Whether to pretty print converted files (boolean)
            - all_rules: Whether to convert all rules (boolean)
            - changed_files: List of changed files (space separated)
            - deleted_files: List of deleted files (space separated)
    """
    parser = argparse.ArgumentParser(
        description="Sigma CLI Conversion",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )

    parser.add_argument(
        "--config",
        help="Path to config YAML file",
        default=os.environ.get("CONFIG", "./config.yaml"),
    )
    parser.add_argument(
        "--path-prefix",
        help="The path prefix to use for input files",
        default=os.environ.get("PATH_PREFIX", os.environ.get("GITHUB_WORKSPACE", ".")),
    )
    parser.add_argument(
        "--render-traceback",
        action=argparse.BooleanOptionalAction,
        help="Render traceback on error",
        default=os.environ.get("RENDER_TRACEBACK", "false").lower() == "true",
    )
    parser.add_argument(
        "--pretty-print",
        action=argparse.BooleanOptionalAction,
        help="Pretty print the converted files",
        default=os.environ.get("PRETTY_PRINT", "false").lower() == "true",
    )
    parser.add_argument(
        "--all-rules",
        action=argparse.BooleanOptionalAction,
        help="Convert all rules",
        default=os.environ.get("ALL_RULES", "false").lower() == "true",
    )

    # File list arguments
    parser.add_argument(
        "--changed-files",
        help="List of changed files",
        default=os.environ.get("CHANGED_FILES", ""),
    )
    parser.add_argument(
        "--deleted-files",
        help="List of deleted files",
        default=os.environ.get("DELETED_FILES", ""),
    )

    return parser.parse_args()


def load_config(config_file: str) -> Dynaconf:
    """
    Load config file.

    Args:
        config_file (str): Path to config YAML file.

    Returns:
        Dynaconf: Config object.
    """
    return Dynaconf(
        envvar_prefix="CONVERT",
        settings_file=[config_file],
        apply_default_on_none=True,
        core_loaders=["YAML"],
    )
