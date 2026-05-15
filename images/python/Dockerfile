# Python agentbox image
FROM python:3.12-slim-bookworm

# Create non-root agent user
RUN groupadd -g 1000 agent && \
    useradd -m -u 1000 -g agent -s /bin/bash agent

# Install common utilities
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    openssh-client \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Install poetry and uv
RUN pip install --no-cache-dir poetry uv

# Install aider-chat
RUN pip install --no-cache-dir aider-chat

# Set up workspace
WORKDIR /workspace
RUN chown agent:agent /workspace

# Switch to agent user
USER agent

# Configure Python for agent user
ENV PYTHONUNBUFFERED=1
ENV PATH="/home/agent/.local/bin:$PATH"

CMD ["/bin/bash"]
