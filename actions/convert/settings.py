"""Settings for the conversion action."""

import argparse
import os

from dynaconf import Dynaconf


def parse_args() -> argparse.Namespace:
    """
    Parse command line arguments to get config file.
    """
    parser = argparse.ArgumentParser(description="Sigma CLI Conversion")
    parser.add_argument(
        "--config",
        dest="config",
        metavar="./config.yaml",
        type=str,
        nargs="?",
        help="Path to config YAML file",
        default=os.environ.get("CONFIG", "./config.yaml"),
        const="./config.yaml",
    )
    parser.add_argument(
        "--conversions-output-dir",
        dest="conversions_output_dir",
        metavar="conversions",
        type=str,
        nargs="?",
        help="Path to output directory for converted files",
        default=os.environ.get("CONVERSIONS_OUTPUT_DIR", "conversions"),
        const="conversions",
    )
    parser.add_argument(
        "--path-prefix",
        dest="path_prefix",
        metavar=".",
        type=str,
        nargs="?",
        help="The path prefix to use for input files",
        default=os.environ.get("GITHUB_WORKSPACE", ""),
        const=".",
    )
    parser.add_argument(
        "--render-traceback",
        dest="render_traceback",
        metavar="true",
        type=str,
        nargs="?",
        help="Render traceback on error",
        default=os.environ.get("RENDER_TRACEBACK", "false") == "true",
        const="true",
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
