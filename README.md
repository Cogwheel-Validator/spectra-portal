<h1 align="center"> The Spectra 💫 Portal</h1>

Spectra Portal is a standalone DApp that extends the functionality of the Spectra Explorer. This DApp
provides an interface that grants users a way to bridge tokens across different chains using the Inter Blockchain Communication Protocol, and swap tokens across different chains using the Osmosis DEX.

## Table of Contents

- [Table of Contents](#table-of-contents)
- [License](#license)
- [Contributing](#contributing)
- [The Idea Behind the App](#the-idea-behind-the-app)
- [What sets The Spectra Portal apart from other solutions?](#what-sets-the-spectra-portal-apart-from-other-solutions)
- [Requirements for a chain and token to be added to the Spectra Portal](#requirements-for-a-chain-and-token-to-be-added-to-the-spectra-portal)
- [How the app works](#how-the-app-works)
  - [The Config Manager](#the-config-manager)
  - [The Pathfinder RPC](#the-pathfinder-rpc)
  - [The Client App](#the-client-app)
- [Prerequisites](#prerequisites)
- [Quickstart for Development](#quickstart-for-development)
- [Deployment and Hosting](#deployment-and-hosting)
  - [Serverless and Long Running Service Deployment](#serverless-and-long-running-service-deployment)
  - [VPS or Bare Metal Deployment](#vps-or-bare-metal-deployment)
  - [Reverse Proxy of the Pathfinder RPC](#reverse-proxy-of-the-pathfinder-rpc)

## License

This project is licensed under the GNU Affero General Public License v3.0. See the [LICENSE](https://github.com/Cogwheel-Validator/spectra-portal/blob/main/LICENSE) file for details.

It is free to use and modify. Any kind of cosmetic changes are allowed but any kind of changes to the codebase responsible for the core functionality needs to be fully open sourced and publicly available.

## Contributing

If you want to add new chain or a new token to the Spectra Portal, you can do so by creating a new issue in the repository and use the templates.

If you want to contribute to the project, please follow the steps below:

1. Fork the repository
2. Create a new branch
3. Make your changes
4. Push your changes to your fork
5. Create a pull request to the main repository
6. Wait for the review and merge the changes

If you are not a developer, you can still contribute to the project by:

1. Reporting bugs
2. Suggesting features
3. Writing documentation

## The Idea Behind the App

Cross-chain bridging and swapping infrastructure in Cosmos currently relies heavily on
centralized APIs. While existing solutions like Skip Go, Squid Router and others work well, they create
potential single points of failure and limit customization options. And there is no option to host or
modify your own variant. This is where the Spectra Portal comes in.

## What sets The Spectra Portal apart from other solutions?

Spectra Portal addresses this by providing:

- **Fully open-source infrastructure** - Every component, from routing logic, RPC server, config manager, and the client app are fully open sourced and publicly available.
- **Self-hostable architecture** - Run your own instance with complete control over data and uptime
- **Transparent development** - All decisions, feature requests, and chain additions managed publicly via GitHub
- **No gatekeeping** - Any legitimate IBC-enabled chain can be added without arbitrary approval processes

## Requirements for a chain and token to be added to the Spectra Portal

For a blockchain and token to be added to the Spectra Portal, they must meet the following requirements:

For a chain:

- Enabled IBC transfers on the chain
- A fully working IBC relayers ( by validators or by the team behind the chain )
- A public RPC and REST endpoints ( on cosmos.directory or available from other validators )
- An explorer where chain data can be shown ( on Mintscan, PingPub or similar )
- An asset that can actually be transferred to other chains ( very rare that this is not the case )

For a token:

- The chain that the token is on must be supported on the Spectra Portal ( see above )
- The token must be transferable to other chains
- The token must have a human readable name and symbol
- The token must have a decimals value
- The token must have a logo ( URL )

**Note on Explorer Integration:** Chains supported by Cogwheel Validator may be
configured to use Spectra Explorer by default to provide a cohesive user experience.
This can be customized in your self-hosted instance or discussed on a case-by-case basis.

## How the app works

The app consists of 3 main components:

1. The frontend client application
2. The backend rpc server
3. The config manager

```mermaid
graph TD;
    D{{"**Config Manager**"}}
    G{{"**Pathfinder RPC Server**"}}
    H{{"**Frontend Client Application**"}}
    J>Chain REST and RPC endpoints];
    A(Chain Registry)-->D;
    B(Keplr Registry)-->D;
    C(Human Made Configs)-->D;
    D==>E[Generated RPC Config];
    D==>F[Generated Client Config];
    E-->G[Pathfinder RPC Server];
    J-->H;
    F-->H;
    G==>H;
```

### The Config Manager

The config manager is a golang checker program that checks the validity and the integrity of the chain
configurations. It goes through the chain configurations and checks that the blockchain endpoints provided are valid and working. It generates the necessary information about each channel and tokens transferred over the
IBC protocol and the necessary information to generate the necessary configuration files for the pathfinder
rpc server and the frontend client application.

### The Pathfinder RPC

The Pathfinder RPC is a golang RPC server made with the ConnectRPC library that provides a service from where
you can query the pathfinder for the best route to bridge tokens between two chains. It also serves as a
information broker for every chain connected via IBC. You can acquire information about all possible
connections between chains and the tokens available on each chain, while it allows the developer to gather
data by using any of the 3 protocols supported by the Pathfinder: gRPC, gRPC-Web, and HTTP-Connect.

For now any kind of swap operation is powered by the Osmosis DEX and the trades are executed directly on the
Osmosis DEX or are executed using Skip Go CosmWasm Smart Contracts. The trade routing is powered by the
[Osmosis SQS](https://github.com/osmosis-labs/sqs). With this Side car query service the Pathfinder is able
to acquire the necessary information to execute the swap operation on the Osmosis DEX. In the future there
might be more options to choose from but for now this is the only option that is available.

While the Pathfinder is great and fully open sourced, it is not perfect and it has some issues and limitations.

| Feature | Spectra's Pathfinder | Skip Go API |
| -------------------- | ------------ | ---------------------------- |
| Open Source | Yes ✅ | Partially ⚠️( Only SDK, and Widget are Open Source ) |
| Fully Customizable | Yes ✅ | Partially ⚠️ |
| Self-Hostable | Yes ✅ | No ❌ |
| Transfer Across Cosmos SDK chains | Mostly ⚠️ ( chains using non-standard slip44 values, will be resolved in the future) | Yes ✅ |
| Usage of multiple DEXs | No ❌(Only Osmosis is supported for now) | Yes ✅ |
| Transfer Assets to non Cosmos SDK chains | No ❌ | Yes ✅ ( through usage of external APIs such as Axelar and custom made implementation for IBC Eureka (ETH) ) |
| Track transaction progress | No ❌ | Yes ✅ |

The Skip API has many advantages and it is more mature. The Pathfinder is still very new and lacks some
features.

### The Client App

The web app is a Next.js application ran with Bun. According by this
[statistics](https://www.statista.com/statistics/1124699/worldwide-developer-survey-most-used-frameworks-web/)
usage of react is above 40% and Next.js is at 20.8%. So it is a very popular framework. Anyone with the
knowledge of React and Typescript should be able to use the app and make changes to it.

The components of the Spectra Portal might have some pros and cons. However the main focus is to provide a
fully open sourced and publicly available app that provides this kind of functionality.

## Prerequisites

To run the Spectra Portal, you need to have the following prerequisites:

- Golang v1.25.5+
- Bun v1.3.9+
- Docker

For development purposes, you might need additional tools and dependencies. However these are not required to
run the Spectra Portal.

- Protoc compilers:
  - protoc-gen-go
  - protoc-gen-connect-go
  - protoc-gen-es
  - protoc-gen-connect-es
  - protoc-gen-ts_proto
- Buf CLI
- Golangci-lint
- Govulncheck
- Snyk CLI
- Semgrep CLI
- Biome
- Make

This also assumes that you are running all of this on some Linux based system. Any commands will assume
you have Debian based OS.

## Quickstart for Development

To get you started with development, you can use the following commands:

```bash
cp rpc-config.toml.example rpc-config.toml
docker compose up -d
```

This will pull the latest images from the repository and start the services.
This is great if you just want to jump in and take the first glance of the project on your system.

If you want to build the images yourself, you can use the docker-compose-dev.yml file to build the images and
start the services.

```bash
docker compose -f docker-compose-dev.yml build
docker compose -f docker-compose-dev.yml up -d
```

This will build the images localy and start the services.

If you do not want to use any sort of config.toml file and want to set up with environment variables, you can edit the following yaml file:

```yaml
services:
  pathfinder:
    environment:
      PATHFINDER_HOST: 0.0.0.0
      PATHFINDER_PORT: 8080
      PATHFINDER_ALLOWED_ORIGINS: "*"
      PATHFINDER_SQS_URLS: "https://sqs.osmosis.zone"
```

This it the base. Every component in from the rpc config file can be replaced with environment variables.
To set every env just use PATHFINDER_ as a prefix and the name of the component as the key in all upper case
letters.

## Deployment and Hosting

**Note** this is more of a suggestion on what you can do and how you can run it. You are after all master
of your own infrastructure and you know best what you need.

To deploy and run this in production you can host it on a VPS behind a reverse proxy. Or use a combination of
some serverless option like Vercel and long running service like AWS App Runner.

The part that stays the same is running the Config Manager. Regardless of your option whenever a chain or a
token is added to the chain_configs directory you need to regenerate the config files. For more info check the
[Config Manager README](config_manager/README.md) but the quickstart is:

```bash
make generate-config
```

This can be automated through some CI/CD pipeline.

### Serverless and Long Running Service Deployment

For **serverless-style** deployment (e.g. frontend on Vercel, Pathfinder on AWS) you can use some AWS,
Vercel any other platform that can host a Next.js application and a long running service. You can use AWS
Lambda for the Pathfinder RPC but I do not recommend it because the client app can make a lot of requests
to the Pathfinder RPC and Lambda could cost you a lot. This are just some suggestions you could always use
something like EC2 or any other VPS provider.

For the RPC you can use something like [App Runner](https://docs.aws.amazon.com/apprunner/).
It runs a single container and it should be easier than managing the whole server. Build the docker image
and push it to the Amazon ECR. Although you should probably modify the dockerfile to include the pathfinder
chain configs and use environment variables for the config.

For the client app something like Vercel is an easy solution. You can also use something like AWS app runner
or some other AWS service that can host a Next.js application.
If you go for the nextjs option you do not need environments with DOCKER_BUILD=true. You should also set
another environment variable NEXT_PUBLIC_PATHFINDER_RPC_URL set to the URL of the Pathfinder RPC.

### VPS or Bare Metal Deployment

If you have a server that you want to run this on you can rely on the docker compose file to run the services,
use orchestrator like Docker Swarm, Kubernetes or Nomad, or keep it simple and run it using SystemD.

For docker setup just use the docker compose file and run it with docker compose up -d with some small
adjustments. You can add something like Watchtower if you have your own docker registry and pull the images
on some changes.

For using something like SystemD make sure to create a dedicated user with no login privileges and no sudo.

```bash
# Create a dedicated service user
sudo adduser --system --no-create-home --shell /bin/false portal

# Clone and setup
sudo mkdir -p /etc/portal
# (assuming you've cloned the repo)
sudo cp -r ./spectra-portal /etc/portal/

# Build
cd /etc/portal
make build-pathfinder
cd client_app && bun install && bun run build && cd ..

# Set ownership
sudo chown -R portal:portal /etc/portal/
sudo chmod -R 755 /etc/portal/
```

Create a systemd service for the pathfinder and the client app.

```bash
sudo tee /etc/systemd/system/portal_app.service > /dev/null <<EOF
[Unit]
Description=Portal Client App"
After=network-online.target

[Service]
User=portal
ExecStart=$(which bun) run start
WorkingDirectory=/etc/portal
Restart=on-failure 
RestartSec=5
LimitNOFILE=8192
Environment="NEXT_PUBLIC_PATHFINDER_RPC_URL=https://pathfinder.thespectra.io"

NoNewPrivileges=true
ProtectSystem=strict
RestrictSUIDSGID=true
LockPersonality=true
PrivateDevices=true
PrivateTmp=true
ProtectControlGroups=yes
ProtectKernelModules=yes
ProtectKernelTunables=yes
RestrictNamespaces=yes

[Install]
WantedBy=multi-user.target
EOF

sudo tee /etc/systemd/system/portal_pathfinder.service > /dev/null <<EOF
[Unit]
Description=Portal Pathfinder RPC"
After=network-online.target

[Service]
User=portal
ExecStart=/etc/portal/build/pathfinder-rpc -config-rpc rpc-config.toml
WorkingDirectory=/etc/portal
Restart=on-failure 
RestartSec=5
LimitNOFILE=8192

NoNewPrivileges=true
ProtectSystem=strict
RestrictSUIDSGID=true
LockPersonality=true
PrivateDevices=true
PrivateTmp=true
ProtectControlGroups=yes
ProtectKernelModules=yes
ProtectKernelTunables=yes
RestrictNamespaces=yes

[Install]
WantedBy=multi-user.target
EOF


sudo systemctl daemon-reload
sudo systemctl enable portal-pathfinder.service
sudo systemctl enable portal-client-app.service
```

This should give you some medium security you can add additional security options here. Adjust the
NEXT_PUBLIC_PATHFINDER_RPC_URL in the client service file to your own Pathfinder RPC URL.

When you are done you can run the services with `sudo systemctl start portal-pathfinder.service` and `sudo systemctl start portal-client-app.service`.

### Reverse Proxy of the Pathfinder RPC

Pathfinder RPC supports 3 protocols out of the box: gRPC, gRPC-Web, and HTTP-Connect. However some of
services won't probably be able to serve all 3. At all times the gRPC-Web and HTTP-Connect should be available.

All of the requests on the RPC are unary so everything should work most of the time.

If you want to support all 3 you can use Nginx like this:

```conf
upstream portal_connect {
    server localhost:8080;
    keepalive 64;
}

upstream portal_frontend {
    server localhost:3000;
    keepalive 64;
}

server {
    listen 443 ssl;
    http2 on;
    server_name pathfinder.example.com;
    include /etc/nginx/ssl.conf;


    location / {
        proxy_pass http://portal_connect;
        
        proxy_http_version 1.1;
        
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        proxy_set_header Connection "";
        proxy_set_header Upgrade $http_upgrade;

        
        proxy_read_timeout 60s;
        proxy_send_timeout 60s;
        proxy_connect_timeout 60s;
        
        proxy_buffering off;
        proxy_request_buffering off;
        
    }
    
    location /server/* {
        deny all;
        return 403;
    }
}

server {
    listen 443 ssl;
    http2 on;

    server_name pathfinder-grpc.example.com;

    include /etc/nginx/ssl.conf;

    charset     utf-8;
    client_max_body_size 15M;

    proxy_buffering off;
    proxy_request_buffering off;

    location  / {
        grpc_pass  grpc://portal_connect;
    }
}

server {
    listen 443 ssl;
    http2 on;
    server_name portal.example.com;
    include /etc/nginx/ssl.conf;
    charset utf-8;
    client_max_body_size 50M;
    
    location / {
        
        proxy_pass http://portal_frontend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
        
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        
        proxy_buffer_size 8k;
        proxy_buffers 16 8k;
        proxy_busy_buffers_size 16k;
    }
}
```

**Note:** Replace `ssl.conf` with your own SSL certificate configuration.

This should give you a basic reverse proxy setup. Any kind of additional security falls upon you. A side note
if you do plan to expand the ConnectRPC to have some sort of streaming Nginx might not be the best for a
reverse proxy. You can check official
[docs](https://connectrpc.com/docs/faq/#how-do-i-proxy-the-connect-protocol-through-nginx) for more
information.
