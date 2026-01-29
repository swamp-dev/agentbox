# Node.js agentbox image
FROM debian:bookworm-slim

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

# Install Node.js 20
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && \
    rm -rf /var/lib/apt/lists/*

# Install pnpm globally
RUN npm install -g pnpm

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

# Set up workspace
WORKDIR /workspace
RUN chown agent:agent /workspace

# Switch to agent user
USER agent

# Configure npm for agent user
RUN mkdir -p /home/agent/.npm && \
    npm config set prefix /home/agent/.npm

ENV PATH="/home/agent/.npm/bin:$PATH"

CMD ["/bin/bash"]
