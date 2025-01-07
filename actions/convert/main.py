import fnmatch
import glob
import os
from pathlib import Path
import traceback

from click.testing import CliRunner
from settings import load_config
from sigma.cli.convert import convert


def convert_files(
    path_prefix: str | Path = Path(os.environ.get("GITHUB_WORKSPACE", "")),
    output_file: str = "-",
    render_tb: bool = False,
):
    """Convert Sigma rules to the target format per each conversion object in the config.

    Args:
        path_prefix (str | Path, optional): The path prefix to use for input files.
            Defaults to GITHUB_WORKSPACE environment variable.
        output_file (str, optional): The output file path. Defaults to "-" (stdout).
        render_tb (bool, optional): Whether to render traceback on error. Defaults to False.

    Raises:
        ValueError: Input file pattern must be relative to the project root.
        ValueError: Invalid input file type.
        ValueError: No files matched the patterns after applying --file-pattern: {file_pattern}.
        ValueError: Pipeline file path must be relative to the project root.
    """
    # Load config from the repository root
    config = load_config()

    # Get default values
    default_target = config.get("defaults.target", "loki")
    default_format = config.get("defaults.format", "default")
    default_skip_unsupported = config.get("defaults.skip-unsupported", "true")
    default_output = config.get("defaults.output", "-")
    default_file_pattern = config.get("defaults.file-pattern", "*.yml")

    # Convert Sigma rules to the target format per each conversion object in the config
    for conversion in config.get("conversions", []):
        print(f"Conversion name: {conversion.get("name", "unnamed conversion")}")
        # Expand input glob patterns
        input_files = []

        # Verify that all input files are relative to the repository root (GITHUB_WORKSPACE)
        input_patterns = conversion.get("input", [])
        if isinstance(input_patterns, str):
            input_patterns = [input_patterns]

        for pattern in input_patterns:
            if Path(pattern).is_absolute():
                raise ValueError(
                    "Input file pattern must be relative to the project root"
                )

        # Expand glob patterns to include all matching files
        match conversion.get("input"):
            case list():
                for pattern in conversion["input"]:
                    input_files.extend(
                        glob.glob(str(path_prefix / pattern), recursive=True)
                    )
            case str():
                input_files.extend(
                    glob.glob(str(path_prefix / conversion["input"]), recursive=True)
                )
            case _:
                raise ValueError("Invalid input file type")

        # Apply file-pattern filtering to exclude files that don't match the pattern
        file_pattern = conversion.get("file-pattern", default_file_pattern)
        filtered_files = [f for f in input_files if fnmatch.fnmatch(f, file_pattern)]

        # Skip conversion if no files match the pattern
        if not filtered_files:
            raise ValueError(
                f"No files matched the patterns after applying --file-pattern: {file_pattern}"
            )

        print(f"Total files: {len(filtered_files)}")
        print(f"Target backend: {conversion.get('target', default_target)}")

        # Verify that all pipeline files are relative to the repository root (GITHUB_WORKSPACE)
        for pipeline in conversion.get("pipelines", []):
            if Path(pipeline).is_absolute():
                raise ValueError(
                    "Pipeline file path must be relative to the project root"
                )

        args = [
            "--target",
            conversion.get("target", default_target),
            *[f"--pipeline={path_prefix / p}" for p in conversion.get("pipelines", [])],
            "--format",
            conversion.get("format", default_format),
            *(
                ["--correlation-method", conversion["correlation-method"]]
                if "correlation-method" in conversion
                and conversion["correlation-method"]
                else []
            ),
            *[f"--filter={f}" for f in conversion.get("filter", [])],
            "--file-pattern",
            file_pattern,
            "--output",
            conversion.get("output", default_output),
            "--encoding",
            conversion.get("encoding", "utf-8"),
            "--json-indent",
            str(conversion.get("json-indent", "0")),
            *[
                f"--backend-option={k}={v}"
                for k, v in conversion.get("backend-option", {}).items()
            ],
            *(
                ["--without-pipeline"]
                if conversion.get("without_pipelines", False)
                else []
            ),
            *(
                ["--disable-pipeline-check"]
                if not conversion.get("pipeline-check", True)
                else []
            ),
            *(
                ["--skip-unsupported"]
                if conversion.get("skip-unsupported", default_skip_unsupported)
                else ["--fail-unsupported"]
            ),
            *filtered_files,
        ]

        runner = CliRunner()
        result = runner.invoke(convert, args=args)

        if result.exception and result.exc_info:
            # If an exception occurred, print the exception and the traceback
            # and the output of the command. We'll continue to run the next conversion.
            print(f"Error during conversion:\n{result.exception}")
            if render_tb:
                trace = "".join(traceback.format_tb(result.exc_info[2]))
                print(f"Traceback:\n{trace}")
            # If an error occurred, print the output of the command. Sometimes the output
            # doesn't contain anything.
            print(f"Output:\n{result.output}".strip())
        else:
            filtered_output = "\n".join(
                line
                for line in result.stdout.splitlines()
                if "Parsing Sigma rules" not in line
            )
            if output_file != "-":
                encoding = conversion.get("encoding", "utf-8")
                with open(output_file, "w", encoding=encoding) as f:
                    f.write(filtered_output.strip())
            else:
                print(filtered_output)

            print(
                f"Converting {conversion.get("name", "unnamed conversion")} completed with exit code"
                f" {result.exit_code}"
            )
            print(f"Output written to {Path(path_prefix / Path(output_file))}")
        print("-" * 80)


if __name__ == "__main__":
    convert_files(output_file="output.txt")
    # exit(1)
