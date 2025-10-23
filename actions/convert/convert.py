"""Convert Sigma rules to the target format per each file in the conversions object
in the config."""

import fnmatch
import glob
import orjson as json
import os
import traceback
from pathlib import Path
from typing import Any

from click.testing import CliRunner
from dynaconf import Dynaconf
from sigma.cli.convert import convert
from yaml import FullLoader, load_all


def convert_rules(
    config: Dynaconf,
    path_prefix: str | Path,
    render_traceback: bool = False,
    pretty_print: bool = False,
    all_rules: bool = False,
    changed_files: str = "",
    deleted_files: str = "",
) -> None:
    """Convert Sigma rules to the target format per each file in the conversions object
    in the config. The converted files will be saved in the PATH_PREFIX/conversions
    directory and they are JSON files with the following structure:
    {
        "queries": [
            "query1",
            "query2",
        ],
        "conversion_name": "conversion_name",
        "input_file": "input_file",
        "rules": [
            {
                "id": "rule_id",
                "title": "rule_title",
                "description": "rule_description",
                "severity": "rule_severity",
                "query": "rule_query",
            },
            {
                "id": "rule_id_2",
                "title": "rule_title_2",
                "description": "rule_description_2",
                "severity": "rule_severity_2",
                "query": "rule_query_2",
            },
        ],
        "output_file": "output_file",
    }

    Args:
        path_prefix (str | Path): The path prefix to use for input files.
        render_traceback (bool): Whether to render traceback on error.
        pretty_print (bool): Whether to pretty print the converted files.
        all_rules (bool): Whether to convert all rules.
        changed_files (str): The list of changed files.
        deleted_files (str): The list of deleted files.

    Raises:
        ValueError: Path prefix must be set using GITHUB_WORKSPACE environment variable.
        ValueError: Conversion output directory is outside the project root.
        ValueError: Conversion name is required and must be a unique identifier
            across all conversion objects in the config.
        ValueError: Input file pattern must be relative to the project root.
        ValueError: Invalid input file type.
        ValueError: No files matched the patterns after applying --file-pattern: {file_pattern}.
        ValueError: Pipeline file path must be relative to the project root.
        ValueError: Error loading rule file {rule_file}.
    """
    changed_files_set = set(
        path_prefix / Path(x) for x in changed_files.split(" ") if x
    )
    deleted_files_set = set(
        path_prefix / Path(x) for x in deleted_files.split(" ") if x
    )

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

    # Check whether we have any files to process
    if not all_rules and not changed_files and not deleted_files:
        print("No changed or deleted files identified, but all_rules is false")
        exit(0)

    # Get the conversion path from the config
    conversion_path = "conversions"  # Default conversion path if none is set
    if folders := config.get("folders"):
        conversion_path = folders.get("conversion_path", conversion_path)
    conversions_output_dir = path_prefix / Path(conversion_path)

    # Check if the conversions_output_dir stays within the project root to prevent path slip.
    if not is_safe_path(path_prefix, conversions_output_dir):
        raise ValueError("Conversion output directory is outside the project root")

    # Create the output directory if it doesn't exist
    conversions_output_dir.mkdir(parents=True, exist_ok=True)

    # Get top-level default values
    default_target = config.get("conversion_defaults.target", "loki")
    default_format = config.get("conversion_defaults.format", "default")
    default_skip_unsupported = config.get("conversion_defaults.skip_unsupported", True)
    default_fail_unsupported = config.get("conversion_defaults.fail_unsupported", False)
    default_encoding = config.get("conversion_defaults.encoding", "utf-8")
    default_pipeline_check = config.get("conversion_defaults.pipeline_check", True)
    default_file_pattern = config.get("conversion_defaults.file_pattern", "*.yml")
    default_correlation_method = config.get(
        "conversion_defaults.correlation_method", []
    )
    default_filters = config.get("conversion_defaults.filters", [])
    default_backend_options = config.get("conversion_defaults.backend_options", {})
    default_without_pipeline = config.get("conversion_defaults.without_pipeline", False)
    default_pipelines = config.get("conversion_defaults.pipelines", [])
    default_json_indent = config.get("conversion_defaults.json_indent", 0)
    default_required_rule_fields = config.get("conversion_defaults.required_rule_fields", [])
    verbose = config.get("verbose", False)

    conversions_to_delete = []
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
                    for deleted_file in deleted_files_set:
                        if fnmatch.fnmatch(
                            str(deleted_file), str(path_prefix / pattern)
                        ):
                            output_filename = f"{name}_{deleted_file.stem}.json"
                            output_path = (
                                path_prefix / conversions_output_dir / output_filename
                            )
                            if output_path.exists():
                                conversions_to_delete.append(output_path)
            case str():
                input_files.extend(
                    glob.glob(str(path_prefix / conversion_input), recursive=True)
                )
                for deleted_file in deleted_files_set:
                    if fnmatch.fnmatch(
                        str(deleted_file), str(path_prefix / conversion_input)
                    ):
                        output_filename = f"{name}_{deleted_file.stem}.json"
                        output_path = (
                            path_prefix / conversions_output_dir / output_filename
                        )
                        if output_path.exists():
                            conversions_to_delete.append(output_path)
            case _:
                raise ValueError("Invalid input file type")

        # Apply file_pattern filtering to exclude files that don't match the pattern
        file_pattern = conversion.get("file_pattern", default_file_pattern)
        filtered_files = [f for f in input_files if fnmatch.fnmatch(f, file_pattern)]

        # Skip conversion if no files match the pattern
        if not filtered_files:
            raise ValueError(
                f"No files matched the patterns after applying file_pattern: {file_pattern}"
            )

        print(f"Total files: {len(filtered_files)}")
        print(f"Target backend: {conversion.get('target', default_target)}")
        if all_rules:
            print("Converting all discovered rules")

        # Verify that all pipeline files are relative to the repository root (GITHUB_WORKSPACE)
        for pipeline in conversion.get("pipelines", default_pipelines):
            if Path(pipeline).is_absolute():
                raise ValueError(
                    "Pipeline file path must be relative to the project root"
                )

        encoding = conversion.get("encoding", default_encoding)

        pipelines = []
        any_pipeline_changed = False
        for pipeline in conversion.get("pipelines", default_pipelines):
            if is_path(pipeline, file_pattern):
                pipeline_path = path_prefix / Path(pipeline)
                pipelines.append(f"--pipeline={pipeline_path}")
                if pipeline_path in changed_files_set:
                    any_pipeline_changed = True
            else:
                pipelines.append(f"--pipeline={pipeline}")

        for input_file in filtered_files:
            # If we're not converting all rules, skip the conversion if:
            # - the file is not in the list of changed files
            # - none of the pipelines have changed
            if (
                not all_rules
                and Path(input_file) not in changed_files_set
                and not any_pipeline_changed
            ):
                print(
                    f"Skipping conversion of {input_file} because it and it's pipelines haven't changed"
                )
                continue

            args = [
                "--target",
                conversion.get("target", default_target),
                *pipelines,
                "--format",
                conversion.get("format", default_format),
                *(
                    ["--correlation-method", conversion["correlation_method"]]
                    if "correlation_method" in conversion
                    and conversion["correlation_method"]
                    else (
                        ["--correlation-method", default_correlation_method]
                        if default_correlation_method
                        else []
                    )
                ),
                *[f"--filter={f}" for f in conversion.get("filters", default_filters)],
                "--file-pattern",
                file_pattern,
                "--output",
                "-",  # Output to stdout, so we can write to a file later
                "--encoding",
                encoding,
                *[
                    f"--backend-option={k}={v}"
                    for k, v in conversion.get(
                        "backend_options", default_backend_options
                    ).items()
                ],
                *(
                    ["--without-pipeline"]
                    if conversion.get("without_pipeline", default_without_pipeline)
                    else []
                ),
                *(
                    ["--disable-pipeline-check"]
                    if not conversion.get("pipeline_check", default_pipeline_check)
                    else ["--pipeline-check"]
                ),
                *(
                    ["--skip-unsupported"]
                    if conversion.get("skip_unsupported", default_skip_unsupported)
                    else (
                        ["--fail-unsupported"]
                        if conversion.get("fail_unsupported", default_fail_unsupported)
                        else []
                    )
                ),
                *(
                    [
                        "--json-indent",
                        str(conversion.get("json_indent")),
                    ]
                    if conversion.get("json_indent")
                    else [
                        "--json-indent",
                        str(default_json_indent),
                    ]
                ),
                *(["--verbose"] if conversion.get("verbose", verbose) else []),
                input_file,
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
                queries = [
                    line
                    for line in result.stdout.splitlines()
                    if "Parsing Sigma rules" not in line and len(line.strip()) != 0
                ]

                if not queries:
                    print("No output generated, skipping writing to file")
                    continue

                # Create output filename based on input file path
                rel_input_path = Path(input_file).relative_to(path_prefix)
                output_filename = f"{name}_{rel_input_path.stem}.json"
                # Replace directory separators with underscores
                output_filename = output_filename.replace(os.sep, "_")
                output_file = path_prefix / conversions_output_dir / output_filename

                # Filter the rules to only include the required fields, if empty, return the full rule dictionaries
                required_rule_fields = conversion.get("required_rule_fields", default_required_rule_fields)

                # Create the output data structure
                output_data = {
                    "queries": queries,
                    "conversion_name": name,
                    "input_file": str(rel_input_path),
                    "rules": filter_rule_fields(load_rules(input_file), required_rule_fields),
                    "output_file": str(Path(output_file).relative_to(path_prefix)),
                }

                # Write the output to a file
                with open(output_file, "w", encoding=encoding) as f:
                    options = json.OPT_NAIVE_UTC | json.OPT_SORT_KEYS
                    if pretty_print:
                        options = options | json.OPT_INDENT_2
                    f.write(
                        json.dumps(
                            output_data,
                            option=options,
                        ).decode(encoding, "blackslashreplace")
                    )

                print(f"Output written to {output_file}")
                print(f"Converting {name} completed with exit code {result.exit_code}")

        print("-" * 80)

    # Remove conversions of deleted rules from the output directory
    if len(conversions_to_delete) > 0:
        print("Removing conversions of deleted rules from the output directory")
        for deleted_file in conversions_to_delete:
            if deleted_file.exists():
                print(f"Removing {deleted_file}")
                os.remove(deleted_file)


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


def load_rules(rule_file: str) -> list[dict[str, Any]]:
    """Load a Sigma rule file(s).

    Args:
        rule_file (str): The path to the Sigma rule file in YAML format.

    Returns:
        Iterator[dict[str, Any]]: The Sigma rule file as a list of dictionaries.

    Raises:
        ValueError: Error loading rule file {rule_file}.
    """

    try:
        with open(rule_file, "r", encoding="utf-8") as f:
            rule_content = f.read()

        # `load_all` is a generator function, hence the use of `list`
        # to enumerate all items.
        return list(load_all(rule_content, FullLoader))
    except Exception as e:
        print(f"{e.__class__.__name__}: Error loading rule file {rule_file}: {str(e)}")
        raise ValueError(f"Error loading rule file {rule_file}") from e

def filter_rule_fields(rule_dicts: list[dict[str, Any]], desired_fields: list[str]) -> list[dict[str, Any]]:
    """Filter the fields of the rules to only include the specified fields.
    If no desired fields are specified, return the full rule dictionaries.

    Args:
        rule_dicts (list[dict[str, Any]]): The list of rule dictionaries.
        desired_fields (list[str]): The list of desired fields. The id and title fields will always be included.

    Returns:
        Iterator[dict[str, Any]]: The filtered Sigma rule files as a list of dictionaries.
    """
    if not desired_fields:
        return rule_dicts
    else:
        # Ensure desired_fields contains at least the id and title fields
        necessary_fields = set(["id", "title"] + desired_fields)

    return [dict((field, rule_dict[field]) for field in necessary_fields if field in rule_dict) for rule_dict in rule_dicts]