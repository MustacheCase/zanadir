import os
import requests
import json
import subprocess

# --- Configuration ---
OPENAI_API_KEY = os.environ.get("OPENAI_API_KEY")
GITHUB_TOKEN = os.environ.get("GITHUB_TOKEN")
REPO = os.environ.get("GITHUB_REPOSITORY")  # e.g., "username/repo"
PR_NUMBER = os.environ.get("GITHUB_REF").split('/')[-1]  # Extract PR number from ref

# --- Functions ---
def get_pr_diff():
    """
    Use git to get the diff of the current pull request.
    In a real scenario, you might use GitHub’s API to fetch the diff.
    """
    try:
        # This command gets the diff between the PR branch and the base branch.
        result = subprocess.run(["git", "diff", "origin/main"], capture_output=True, text=True)
        return result.stdout
    except Exception as e:
        print("Error getting diff:", e)
        return ""

def generate_review(diff_text):
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
    
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {OPENAI_API_KEY}"
    }
    data = {
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": prompt}],
        "max_tokens": 2000, 
    }
    
    response = requests.post("https://api.openai.com/v1/chat/completions", headers=headers, json=data)
    if response.status_code == 200:
        review = response.json()['choices'][0]['message']['content']
        return review
    else:
        print("OpenAI API error:", response.text)
        return "Error generating review."

def post_comment(review_text):
    url = f"https://api.github.com/repos/{REPO}/issues/{PR_NUMBER}/comments"
    headers = {
        "Authorization": f"token {GITHUB_TOKEN}",
        "Accept": "application/vnd.github+json"
    }
    data = {
        "body": review_text
    }
    response = requests.post(url, headers=headers, data=json.dumps(data))
    if response.status_code in [200, 201]:
        print("Review comment posted successfully.")
    else:
        print("Failed to post review comment:", response.text)

# --- Main Workflow ---
if __name__ == "__main__":
    diff = get_pr_diff()
    if not diff.strip():
        print("No diff found to review.")
        exit(0)
    
    review = generate_review(diff)
    post_comment(review)
