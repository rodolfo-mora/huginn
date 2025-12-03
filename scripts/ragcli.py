#!/usr/bin/env python3
import requests
import json
import sys
import re

def generate_embedding(text):
    """Generate embedding for the query text using Ollama"""
    response = requests.post(
        "http://localhost:11434/api/embeddings",
        json={
            "model": "mxbai-embed-large",  # Use 1024-dimensional model to match Qdrant
            "prompt": text
        }
    )
    return response.json()["embedding"]

def query_qdrant(query_text):
    """Search Qdrant for relevant anomalies using query embedding"""
    # Generate embedding for the query
    query_vector = generate_embedding(query_text)
    
    # Search Qdrant for relevant anomalies
    response = requests.post(
        "http://localhost:6333/collections/alerts/points/search",
        json={
            "vector": query_vector,
            "limit": 1000,
            "with_payload": True,
            "score_threshold": 0.6  # LOWERED from 0.6 to 0.4
        }
    )
    return response.json()

def strip_think_tags(text):
    """Remove any content enclosed in <think>...</think> tags from the text."""
    if not isinstance(text, str):
        return text
    # Remove well-formed <think>...</think> blocks (case-insensitive, multiline)
    cleaned = re.sub(r'(?is)<think>.*?</think>', '', text)
    # If a closing tag is missing, drop everything from <think> to end of string
    cleaned = re.sub(r'(?is)<think>.*\Z', '', cleaned)
    return cleaned.strip()

def query_ollama(prompt, template=None):
    # Apply template if provided
    if template:
        prompt = template.format(prompt=prompt)
    
    response = requests.post(
        "http://localhost:11434/api/generate",
        # "http://10.96.4.90:8080/api/generate",
        json={
            "model": "qwen3:8b",  # Use a more capable model
            "prompt": prompt,
            "stream": False
        }
    )
    try:
        return strip_think_tags(response.json()["response"])
    except:
        print(f"Error: {response.json()}")
        return "Error: " + str(response.json())
    return response.json()["response"]

# Predefined templates
TEMPLATES = {
    "json": """You are a Kubernetes expert analyzing anomalies. Respond in valid JSON format.

{prompt}

Respond with a JSON object containing:
- summary: Brief overview
- critical_issues: Array of critical problems
- recommendations: Array of actionable recommendations
- severity_levels: Object with high/medium/low counts""",

    "markdown": """You are a Kubernetes expert analyzing anomalies. Respond in Markdown format.

{prompt}

Format your response with:
- ## Summary
- ## Critical Issues
- ## Recommendations
- ## Severity Breakdown""",

    "table": """You are a Kubernetes expert analyzing anomalies. Respond with a structured table.

{prompt}

Create a table with columns:
| Issue | Severity | Impact | Recommendation |""",

    "bullet": """You are a Kubernetes expert analyzing anomalies. Use bullet points for clarity.

{prompt}

Structure your response with:
• Summary
• Critical Issues (bullet points)
• Recommendations (bullet points)"""
}

# Usage
if len(sys.argv) > 1:
    question = " ".join(sys.argv[1:])
    # print(f"Querying for: {question}")
    
    # Check for template argument
    template_name = None
    if "--template" in sys.argv:
        template_index = sys.argv.index("--template")
        if template_index + 1 < len(sys.argv):
            template_name = sys.argv[template_index + 1]
            # Remove template args from question
            question = " ".join(sys.argv[1:template_index])
    
    # Get relevant context from Qdrant
    context_data = query_qdrant(question)
    
    # Extract anomaly descriptions from the results
    anomalies = []
    kafka_kraft_issues = []  # Separate kafka-kraft issues
    # print(f"context_data: {context_data} \n\n")
    if 'result' in context_data and context_data['result']:
        for point in context_data['result']:
            payload = point.get('payload', {})
            # Prefer richer encoding text if present in metadata
            metadata = payload.get('metadata', {}) if isinstance(payload.get('metadata'), dict) else {}
            rich = metadata.get('encodingText')
            if isinstance(rich, str) and rich.strip():
                # Prefix cluster if available to maintain cluster context
                cluster = payload.get('cluster')
                anomalies.append(f"[{cluster}] {rich}" if cluster else rich)
                continue

            if 'description' in payload:
                description = payload['description']
                # Add cluster information if available
                if 'cluster' in payload and payload['cluster']:
                    cluster = payload['cluster']
                    severity = payload['severity']
                    type = payload['type']
                    resourcetype = payload['resourcetype']
                    resource = payload['resource']
                    nodename = payload['nodename']
                    namespace = payload['namespace']
                    description = f"Anomaly on cluster {cluster} of severity {severity} and type {type} in {resourcetype} name {resource} on node {nodename} on namespace {namespace}, alert description: {description}"
                anomalies.append(description)
    
    context = "\n".join(anomalies)
    
    if not context or context == "":
        context = "No relevant anomalies found."
    
    # print(f"context: {context} \n\n")
    # Create prompt with context
    prompt = f"""You are a Kubernetes expert analyzing anomalies. Based on these Kubernetes logs and events:

{context}

Question: {question}

Please respond in a way that is easy to understand and follow, avoid over explaining and do not deviate from the facts. Make suggestions for improvements to the Kubernetes cluster."""

    # Get template
    template = TEMPLATES.get(template_name) if template_name else None
    
    answer = query_ollama(prompt, template)
    print(answer)
else:
    print("Usage: python ragcli.py 'your question here' [--template format]")
    print("Available templates: json, markdown, table, bullet")
    print("Example: python ragcli.py 'What are the most critical issues?' --template json")

# IMPORTANT: Pay special attention to any entries mentioning "kafka-kraft", "Secret", or "controller-*-id" as these are highly relevant to the question.