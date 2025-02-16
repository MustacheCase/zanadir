import os
import json
import requests  # still used for posting GitHub comments

# Import the Gemini SDK
from google import genai
from google.genai import types

# --- Configuration ---
REPO_OWNER = os.environ.get("GITHUB_REPOSITORY_OWNER")
GEMINI_API_KEY = os.environ.get("GEMINI_API_KEY")
GITHUB_TOKEN = os.environ.get("GITHUB_TOKEN")
REPO = os.environ.get("GITHUB_REPOSITORY")  # e.g., "username/repo"
PR_NUMBER = os.environ.get("PR_NUMBER")

# --- Functions ---
def fetch_pr_diff(repo_owner, repo_name, pr_number):
    """
    Use GitHub to get the diff of the current pull request.
    """
    print("REPO_OWNER: " + repo_owner)
    print("REPO: " + repo_name)
    print("PR_NUMBER: " + pr_number)
    url = f"https://github.com/{repo_owner}/{repo_name}/pull/{pr_number}.diff"
    response = requests.get(url)
    if response.status_code == 200:
        return response.text
    else:
        raise Exception(f"Failed to fetch PR diff: {response.status_code}, {response.text}")

def generate_review(diff_text):
    # Build the prompt with detailed review instructions.
    prompt = (
        "You are a GO code review assistant. Your task is to review the following code diff with a focus on these areas:\n\n"
        "1. Code Quality:\n"
        "   - Identify bugs, logic errors, and edge cases\n"
        "   - Highlight performance and optimization opportunities\n"
        "   - Check error handling, input validation, and naming consistency\n"
        "   - Verify type safety and numeric range constraints\n\n"
        "2. Security:\n"
        "   - Flag potential vulnerabilities and improper input sanitization\n"
        "   - Verify authentication/authorization handling and check for exposed sensitive data\n\n"
        "3. Best Practices:\n"
        "   - Assess adherence to coding standards and DRY principles\n"
        "   - Evaluate commenting, documentation, and test coverage\n"
        "   - Ensure consistency across similar objects\n\n"
        "4. Architecture & Design:\n"
        "   - Consider impact on existing architecture and scalability\n"
        "   - Check separation of concerns and any API contract changes\n\n"
        "Please provide clear, concise, and actionable feedback.\n\n"
        "Code Diff:\n"
        f"{diff_text}\n\n"
        "Review:"
    )
    
    # Initialize the Gemini client using your API key.
    client = genai.Client(api_key=GEMINI_API_KEY)
    
    # Generate the review using the Gemini API.
    response = client.models.generate_content(
        model="gemini-2.0-flash",
        contents=prompt)
    return response.text

def post_comment(review_text):
    url = f"https://api.github.com/repos/{REPO}/issues/{PR_NUMBER}/comments"
    headers = {
        "Authorization": f"token {GITHUB_TOKEN}",
        "Accept": "application/vnd.github+json"
    }
    data = {"body": review_text}
    response = requests.post(url, headers=headers, data=json.dumps(data))
    if response.status_code in [200, 201]:
        print("Review comment posted successfully.")
    else:
        print("Failed to post review comment:", response.text)

# --- Main Workflow ---
if __name__ == "__main__":
    diff = fetch_pr_diff(REPO_OWNER, REPO.split('/')[-1], PR_NUMBER)
    if not diff.strip():
        print("No diff found to review.")
        exit(0)
    
    review = generate_review(diff)
    post_comment(review)
