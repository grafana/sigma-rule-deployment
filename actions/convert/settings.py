import argparse
from dynaconf import Dynaconf


def parse_args():
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
        default="./config.yaml",
        const="./config.yaml",
    )
    parser.add_argument(
        "--conversions-output-dir",
        dest="conversions_output_dir",
        metavar="conversions",
        type=str,
        nargs="?",
        help="Path to output directory for converted files",
        default="conversions",
        const="conversions",
    )
    parser.add_argument(
        "--render-traceback",
        dest="render_traceback",
        metavar="true",
        type=str,
        nargs="?",
        help="Render traceback on error",
        default="false",
        const="true",
    )
    return parser.parse_args()


def load_config():
    """
    Load config file.
    """
    args = parse_args()
    return Dynaconf(
        envvar_prefix="CONVERT",
        settings_file=[args.config],
        apply_default_on_none=True,
        core_loaders=["YAML"],
    )
