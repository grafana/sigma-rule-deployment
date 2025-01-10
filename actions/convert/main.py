"""Main entrypoint for the convert script."""

from convert import convert_rules
from settings import load_config, parse_args

if __name__ == "__main__":
    # Parse command line arguments
    args = parse_args()
    config = load_config(args.path_prefix / args.config)

    # Get the conversions output directory from the config file.
    conversions_output_dir = config.get("folders.conversion_path", "conversions")

    # Convert Sigma rules to the target format per each conversion object in the config
    convert_rules(
        config=config,
        path_prefix=args.path_prefix,
        conversions_output_dir=conversions_output_dir,
        render_traceback=args.render_traceback,
    )
