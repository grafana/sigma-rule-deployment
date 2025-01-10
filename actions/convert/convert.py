"""Convert Sigma rules to the target format per each conversion object in the config."""

import fnmatch
import glob
import os
from pathlib import Path
import shutil
import traceback

from dynaconf import Dynaconf
from click.testing import CliRunner
from sigma.cli.convert import convert


def convert_rules(
    config: Dynaconf,
    path_prefix: str | Path = Path(os.environ.get("GITHUB_WORKSPACE", "")),
    conversions_output_dir: str | Path = Path(
        os.environ.get("CONVERSIONS_OUTPUT_DIR", "conversions")
    ),
    render_traceback: bool = os.environ.get("RENDER_TRACEBACK", "false").lower()
    == "true",
) -> None:
    """Convert Sigma rules to the target format per each conversion object in the config.

    Args:
        path_prefix (str | Path, optional): The path prefix to use for input files.
            Defaults to GITHUB_WORKSPACE environment variable.
        output_file (str, optional): The output file path. Defaults to "-" (stdout).
        render_tb (bool, optional): Whether to render traceback on error. Defaults to False.

    Raises:
        ValueError: Path prefix must be set using GITHUB_WORKSPACE environment variable.
        ValueError: Conversion output directory is outside the project root.
        ValueError: Conversion name is required and must be a unique identifier
            across all conversion objects in the config.
        ValueError: Input file pattern must be relative to the project root.
        ValueError: Invalid input file type.
        ValueError: No files matched the patterns after applying --file-pattern: {file_pattern}.
        ValueError: Pipeline file path must be relative to the project root.
    """
    # Check if the path_prefix is set
    if not path_prefix or path_prefix == Path("."):
        raise ValueError(
            "Path prefix must be set using GITHUB_WORKSPACE environment variable."
        )

    # Convert path_prefix to a Path object if it's a string.
    # If it's already a Path object, it will remain unchanged.
    path_prefix = Path(path_prefix)

    # Resolve the path_prefix to an absolute path
    if not path_prefix.is_absolute():
        path_prefix = path_prefix.resolve()

    # Check if the conversions_output_dir stays within the project root to prevent path slip.
    conversions_output_dir = path_prefix / Path(conversions_output_dir)
    if not is_safe_path(path_prefix, conversions_output_dir):
        raise ValueError("Conversion output directory is outside the project root")

    # Remove the output directory if it exists
    if conversions_output_dir.is_dir():
        shutil.rmtree(conversions_output_dir)

    # Create the output directory if it doesn't exist
    conversions_output_dir.mkdir(parents=True, exist_ok=True)

    # Get top-level default values
    default_target = config.get("defaults.target", "loki")
    default_format = config.get("defaults.format", "default")
    default_skip_unsupported = config.get("defaults.skip-unsupported", "true")
    default_file_pattern = config.get("defaults.file-pattern", "*.yml")

    # Convert Sigma rules to the target format per each conversion object in the config
    for conversion in config.get("conversions", []):
        # If the conversion name is not unique, we'll overwrite the output file,
        # which might not be the desired behavior for the user.
        name = conversion.get("name", None)
        if not name:
            raise ValueError(
                "Conversion name is required and must be a unique identifier"
                " across all conversion objects in the config"
            )
        print(f"Conversion name: {name}")

        # Verify that all input files are relative to the repository root (GITHUB_WORKSPACE)
        input_patterns = conversion.get("input", [])
        if isinstance(input_patterns, str):
            input_patterns = [input_patterns]

        for pattern in input_patterns:
            if Path(pattern).is_absolute():
                raise ValueError(
                    "Input file pattern must be relative to the project root"
                )

        # Expand glob patterns to include all matching files only
        input_files = []
        conversion_input = conversion.get("input", None)
        match conversion_input:
            case list():
                for pattern in conversion_input:
                    input_files.extend(
                        glob.glob(str(path_prefix / pattern), recursive=True)
                    )
            case str():
                input_files.extend(
                    glob.glob(str(path_prefix / conversion_input), recursive=True)
                )
            case _:
                raise ValueError("Invalid input file type")

        # Apply file-pattern filtering to exclude files that don't match the pattern
        file_pattern = conversion.get("file-pattern", default_file_pattern)
        filtered_files = [f for f in input_files if fnmatch.fnmatch(f, file_pattern)]

        # Skip conversion if no files match the pattern
        if not filtered_files:
            raise ValueError(
                f"No files matched the patterns after applying file-pattern: {file_pattern}"
            )

        print(f"Total files: {len(filtered_files)}")
        print(f"Target backend: {conversion.get('target', default_target)}")

        # Verify that all pipeline files are relative to the repository root (GITHUB_WORKSPACE)
        for pipeline in conversion.get("pipelines", []):
            if Path(pipeline).is_absolute():
                raise ValueError(
                    "Pipeline file path must be relative to the project root"
                )

        # Output file path
        output_file = path_prefix / conversions_output_dir / Path(f"{name}.txt")

        encoding = conversion.get("encoding", "utf-8")

        pipelines = []
        for pipeline in conversion.get("pipelines", []):
            if is_path(pipeline, file_pattern):
                pipelines.append(f"--pipeline={path_prefix / Path(pipeline)}")
            else:
                pipelines.append(f"--pipeline={pipeline}")

        args = [
            "--target",
            conversion.get("target", default_target),
            *pipelines,
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
            "-",  # Output to stdout, so we can write to a file later
            "--encoding",
            encoding,
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
            if render_traceback:
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

            if not filtered_output:
                print("No output generated, skipping writing to file")
                continue

            with open(output_file, "w", encoding=encoding) as f:
                f.write(filtered_output.strip())

            print(f"Converting {name} completed with exit code" f" {result.exit_code}")
            print(f"Output written to {path_prefix / Path(output_file)}")
        print("-" * 80)


def is_safe_path(base_dir: str | Path, target_path: str | Path) -> bool:
    """
    Check if the target_path is within the base_dir (to prevent path slip).

    Args:
        base_dir (str | Path): The base directory.
        target_path (str | Path): The target path to check.

    Returns:
        bool: True if target_path is within base_dir, False otherwise.
    """
    base_dir = Path(base_dir).resolve()
    target_path = Path(target_path).resolve()

    return base_dir in target_path.parents or base_dir == target_path


def is_path(path_string, file_pattern) -> bool:
    """Check if the string is a valid path.

    Args:
        path_string (str): The string to check.
        file_pattern (str): The file pattern to match, like "*.yml".

    Returns:
        bool: True if the string is a valid path, False otherwise.
    """
    if os.path.exists(path_string):
        return True

    if Path(path_string).is_absolute() or "/" in path_string:
        return True

    if os.path.splitext(path_string)[1] and fnmatch.fnmatch(path_string, file_pattern):
        return True

    return False
