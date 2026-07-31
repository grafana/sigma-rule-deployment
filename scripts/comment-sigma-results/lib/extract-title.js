import fs from 'fs';
import path from 'path';

/**
 * Extract title from JSON file
 */
export function extractTitle(filePath) {
  try {
    // Get the root of the repository - this is used to resolve the absolute path to the file when the scripts runs from a different repo
    const repoRoot = process.env.RULE_DIRECTORY_PATH || process.cwd();

    const absolutePath = path.isAbsolute(filePath)
      ? filePath
      : path.join(repoRoot, filePath);

    if (!fs.existsSync(absolutePath)) {
      console.log(`File does not exist: ${absolutePath}`);
      return path.basename(filePath);
    }

    const content = fs.readFileSync(absolutePath, 'utf8');

    // Try JSON parsing first
    try {
      const jsonData = JSON.parse(content);

      // Check for title at top level (for alert rule files)
      if (jsonData.title && typeof jsonData.title === 'string') {
        return jsonData.title.trim();
      }

      // Check for title in rules array (for conversion output files)
      if (jsonData.rules && Array.isArray(jsonData.rules) && jsonData.rules.length > 0) {
        const firstRule = jsonData.rules[0];
        if (firstRule && firstRule.title && typeof firstRule.title === 'string') {
          return firstRule.title.trim();
        }
      }
    } catch (jsonError) {
      // JSON parsing failed, will try regex fallback
      console.log(`JSON parsing failed for ${filePath}: ${jsonError.message}, trying regex fallback`);
    }

    // Fallback to regex if JSON parsing didn't find title
    const titleMatch = content.match(/"title":\s*"([^"]+)"/);
    if (titleMatch && titleMatch[1]) {
      return titleMatch[1].trim();
    }

    // Final fallback to filename if no title found
    console.log(`No title found in ${filePath} using JSON or regex`);
    return path.basename(filePath);
  } catch (error) {
    console.log(`Error reading file ${filePath}: ${error.message}`);
    // Fallback to filename if file can't be read
    return path.basename(filePath);
  }
}
