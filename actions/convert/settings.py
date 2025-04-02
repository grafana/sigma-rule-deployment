"""Settings for the conversion action."""

import argparse
import os

from dynaconf import Dynaconf


TRUE_VALUES = ("true", "1", "t", "yes", "y", "on", "enabled")


def parse_args() -> argparse.Namespace:
    """
    Parse command line arguments to get config file.

    Order of precedence for configuration values:
    1. Command line arguments (highest priority)
    2. Environment variables
    3. Default values (lowest priority)

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
        default=os.environ.get("CONFIG_PATH", "./config.yaml"),
    )
    parser.add_argument(
        "--path-prefix",
        help="The path prefix to use for input files",
        default=os.environ.get("PATH_PREFIX", os.environ.get("GITHUB_WORKSPACE", ".")),
    )
    parser.add_argument(
        "--render-traceback",
        help="Render traceback on error",
        default=os.environ.get("RENDER_TRACEBACK", "false").lower() in TRUE_VALUES,
    )
    parser.add_argument(
        "--pretty-print",
        help="Pretty print the converted files",
        default=os.environ.get("PRETTY_PRINT", "false").lower() in TRUE_VALUES,
    )
    parser.add_argument(
        "--all-rules",
        help="Convert all rules",
        default=os.environ.get("ALL_RULES", "false").lower() in TRUE_VALUES,
    )
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
