"""Main entrypoint for the convert script."""

from convert import convert_rules
from settings import load_config, parse_args

if __name__ == "__main__":
    # Parse command line arguments
    args = parse_args()
    # Convert Sigma rules to the target format per each conversion object in the config
    convert_rules(
        config=load_config(args.config),
        path_prefix=args.path_prefix,
        conversions_output_dir=args.conversions_output_dir,
        render_traceback=args.render_traceback,
    )
