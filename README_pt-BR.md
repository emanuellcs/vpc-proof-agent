# VPC Proof Agent

Um **agente de diagnóstico e coleta de evidências** escrito em Go, que valida e comprova, tecnicamente, que um ambiente de rede AWS provisionado manualmente está funcionando corretamente.

O agente executa em uma instância EC2 (Amazon Linux 2) e valida um ambiente-alvo composto por uma VPC (`10.0.0.0/16`), uma sub-rede pública (`10.0.1.0/24` com atribuição automática de IP público), um Internet Gateway, uma tabela de rotas com rota padrão para o IGW, um Security Group e uma instância EC2 (`t2.micro`).

> **O agente NÃO provisiona recursos AWS.** Ele parte do princípio de que o ambiente existe e o valida, gerando evidências reproduzíveis.

- **CLI**: administração, diagnósticos aprofundados e geração de relatórios, usado localmente via SSH.
- **REST API (v1)**: exposta publicamente para comprovar acessibilidade pela internet, roteamento externo e configuração do Security Group.

---

## Sumário

- [Relatório Acadêmico: Validação da Infraestrutura AWS](#relatório-acadêmico-validação-da-infraestrutura-aws)
- [Como o agente valida o laboratório](#como-o-agente-valida-o-laboratório)
- [Visão técnica](#visão-técnica)
- [Como executar localmente](#como-executar-localmente)
- [Segurança](#segurança)
- [Documentação](#documentação)

---

## Relatório Acadêmico: Validação da Infraestrutura AWS

### 1. Introdução

Este projeto foi desenvolvido para auxiliar na entrega da atividade final do módulo intermediário da trilha de Provimento de Serviços Computacionais (PSC) do Capacita iRede. O software atua como um agente de validação técnica, servindo exclusivamente para comprovar e auditar se todos os requisitos de infraestrutura AWS exigidos no laboratório foram configurados e conectados corretamente.

O laboratório consiste em uma rede virtual provisionada manualmente na AWS: uma VPC com CIDR `10.0.0.0/16`, uma sub-rede pública `10.0.1.0/24` com atribuição automática de IP público, um Internet Gateway anexado, uma tabela de rotas com rota padrão `0.0.0.0/0` apontando para o IGW, um Security Group liberando SSH (porta 22) e HTTP (porta 8080), e uma instância EC2 `t2.micro`. O papel do agente é verificar, por meio de sondas técnicas, que cada um desses componentes existe e está corretamente interligado, transformando o resultado em um relatório de evidências.

### 2. Metodologia

O agente executa **nove sondas** de rede e de sistema. Cada sonda produz um resultado (**Pass**, **Fail** ou **Warn**, respectivamente aprovado, falhou ou advertência) e registra detalhes técnicos. Os resultados alimentam um **motor de diagnóstico** baseado em regras, que traduz falhas em dicas acionáveis de solução de problemas específicas da AWS. Por fim, o agente gera **relatórios de evidências** nos formatos JSON, Markdown e texto puro; cada relatório carrega um **hash de integridade SHA-256**, garantindo que o documento não foi adulterado após a geração.

### 3. Mapeamento dos Recursos às Sondas

A tabela a seguir associa cada recurso do laboratório à sonda que o valida e ao tipo de evidência produzida.

| Recurso do laboratório | Sonda(s) responsável(is) | Evidência produzida |
| --- | --- | --- |
| VPC `10.0.0.0/16` | `vpc_ownership` | O IP privado da instância pertence ao CIDR da VPC |
| Sub-rede Pública `10.0.1.0/24` | `subnet_ownership` + `public_ip_consistency` | O IP privado pertence ao CIDR da sub-rede e há IP público atribuído |
| Internet Gateway (IGW) | `internet_https` | O tráfego HTTPS de saída consegue deixar a VPC |
| Tabela de Rotas `0.0.0.0/0 → IGW` | `default_route` + `internet_https` | Existe rota padrão ativa no sistema operacional e o roteamento funciona na rede AWS |
| Security Group (SSH/22 e HTTP/8080) | Acesso SSH (CLI) + HTTP externo ao `/api/v1/echo` | A porta 22 aceita acesso administrativo e a porta 8080 recebe requisições externas |
| Instância EC2 `t2.micro` | `metadata` (IMDSv2) + `system_resources` | A instância responde ao IMDSv2 e apresenta recursos de sistema saudáveis |
| Acessibilidade pela internet | `public_ip_consistency` + `echo` | O IP público reportado pela AWS coincide com o IP observado externamente |

#### 3.1 VPC (10.0.0.0/16): sonda `vpc_ownership`

A sonda obtém o IP privado da instância por meio do IMDSv2 e verifica, por cálculo de CIDR, se esse endereço pertence à faixa `10.0.0.0/16`. Um resultado **Pass** comprova que a instância foi lançada dentro da VPC correta; um **Fail** indica que a instância pode estar em outra VPC ou que o CIDR esperado está incorreto.

#### 3.2 Sub-rede Pública (10.0.1.0/24): sondas `subnet_ownership` e `public_ip_consistency`

A sonda `subnet_ownership` confirma que o IP privado está dentro de `10.0.1.0/24`, enquanto a sonda `public_ip_consistency` compara o IP público informado pela AWS (IMDSv2) com o IP público observado por um serviço externo de eco. Juntas, elas comprovam que a instância está na sub-rede pública e que a atribuição automática de IP público está habilitada.

#### 3.3 Internet Gateway (IGW): sonda `internet_https`

A sonda `internet_https` realiza uma requisição HTTPS para um serviço externo (ex.: `https://checkip.amazonaws.com`). O sucesso dessa requisição prova, de forma conclusiva e indireta, que o Internet Gateway está anexado à VPC e que o tráfego de saída consegue alcançar a internet.

#### 3.4 Tabela de Rotas (0.0.0.0/0 → IGW): sondas `default_route` e `internet_https`

A sonda `default_route` lê a tabela de rotas do sistema operacional (`/proc/net/route`) e confirma a existência de uma rota padrão ativa com gateway. Combinada com o sucesso da sonda `internet_https`, ela comprova a cadeia completa: a rota `0.0.0.0/0` aponta para o IGW e está associada à sub-rede.

#### 3.5 Security Group (SSH/22 e HTTP/8080)

O Security Group é validado pelo **sucesso operacional** dos acessos externos: (1) o acesso administrativo via **SSH** (porta 22) a partir do endereço IP autorizado demonstra que a regra de entrada está liberando o tráfego administrativo; e (2) requisições HTTP externas alcançando o endpoint `/api/v1/echo` (porta 8080) demonstram que a regra de entrada do serviço público está ativa.

#### 3.6 Instância EC2 (t2.micro): sondas `metadata` e `system_resources`

A sonda `metadata` acessa o serviço de metadados da instância via **IMDSv2** (obtendo ID da instância, IPs e zona de disponibilidade), comprovando que a instância existe e que o serviço de metadados está acessível. A sonda `system_resources` lê `/proc/uptime`, `/proc/loadavg` e `/proc/meminfo`, verificando que a instância possui memória disponível e carga de processamento dentro de limites saudáveis.

#### 3.7 Acessibilidade pela internet

A combinação das sondas `public_ip_consistency` e `echo` comprova que a instância possui um endereço IP público válido e que esse endereço é alcançável pela internet, fechando o ciclo de validação do laboratório.

### 4. Evidências e Interpretação

Para gerar as evidências, o avaliador pode executar:

```bash
vpc-proof check        # verifica todas as sondas; código de saída 0 (ok), 1 (falha), 2 (advertência)
vpc-proof diagnose     # apresenta as sondas e as dicas de solução de problemas
vpc-proof report --format markdown --output evidence.md
```

O relatório Markdown gerado contém: resumo do agente, seção da instância, resumo de rede, tabela de resultados das sondas (ID, status, duração e mensagem), dicas de diagnóstico e o **hash de integridade SHA-256**. Esse relatório é a principal evidência a ser entregue. Além disso, o endpoint público `GET /api/v1/echo` da REST API reflete o endereço IP de quem o acessa, servindo como prova de alcançabilidade externa e de correta configuração do Security Group.

### 5. Conclusão

O VPC Proof Agent demonstra, por meio de evidências técnicas reproduzíveis, que a infraestrutura AWS provisionada manualmente atende a todos os requisitos do laboratório: a VPC, a sub-rede pública, o Internet Gateway, a tabela de rotas, o Security Group e a instância EC2 estão corretamente criados e interligados. Cada recurso é validado por uma sonda específica, e o relatório final comprova o funcionamento da rede com rigor acadêmico.

---

## Como o agente valida o laboratório

O objetivo central é comprovar, com evidências técnicas, que o ambiente de rede AWS foi configurado corretamente. O agente:

1. **Extrai os metadados da instância** via IMDSv2 (ID da instância, IP privado, IP público e zona de disponibilidade).
2. **Executa sondas de rede e de sistema** para verificar o pertencimento ao CIDR da VPC/sub-rede, a rota padrão e o gateway, a resolução de DNS, a conectividade HTTPS de saída, os recursos do sistema e a sincronização de relógio.
3. **Compara o IP público** reportado pela AWS com o IP público observado por um serviço de eco externo.
4. **Traduz falhas** em dicas acionáveis de troubleshooting AWS.
5. **Gera relatórios** de evidências em JSON, Markdown e texto puro, com hash de integridade SHA-256.

O agente funciona por duas interfaces complementares:

- **CLI**: utilizada por um avaliador/administrador via SSH para diagnósticos profundos e geração de relatórios.
- **REST API**: exposta publicamente (porta 8080) para provar que a instância é alcançável pela internet, que o roteamento externo funciona e que o Security Group está liberando o tráfego corretamente. Inclui endpoints de saúde, informações, sondas, relatório, histórico, configuração e especificação OpenAPI.

## Visão técnica

- **Linguagem:** Go 1.26
- **Arquitetura:** Clean Architecture com o layout padrão de projetos Go (`cmd/`, `internal/`, `pkg/`).
- **Mock local da AWS:** LocalStack (acessado via CLI `lstk` ou `aws` com endpoints customizados).
- **Provisionamento do laboratório:** script idempotente em `scripts/setup-localstack.sh`, que cria VPC, sub-rede, IGW, tabela de rotas e Security Group dentro do LocalStack usando a AWS CLI.
- **Observabilidade:** logging estruturado (JSON/Texto), IDs de requisição e métricas no formato Prometheus.
- **Segurança:** autenticação por token, limitador de requisições por IP, suporte a TLS e hash de integridade nos relatórios.

## Como executar localmente

Pré-requisitos: Go 1.26, LocalStack em execução, AWS CLI v2, Make e golangci-lint v2.

```bash
# 1. Iniciar o LocalStack
lstk start

# 2. Provisionar o laboratório AWS dentro do LocalStack
make localstack-setup

# 3. Compilar e testar
make build
make test
make lint
make e2e

# 4. Executar diagnósticos
./bin/vpc-proof check
./bin/vpc-proof report --format markdown --output evidence.md
./bin/vpc-proof serve
```

Para remover o laboratório provisionado:

```bash
make localstack-teardown
```

## Segurança

- O agente **nunca** provisiona recursos e **nunca** lê ou expõe credenciais AWS.
- A REST API é protegida por autenticação baseada em token e por limitador de requisições por IP.
- Os scripts do LocalStack **forçam credenciais fictícias** (`AWS_ACCESS_KEY_ID=test`, `AWS_SECRET_ACCESS_KEY=test`) e um endpoint dedicado, garantindo que nunca afetem uma conta AWS real, mesmo que o avaliador possua credenciais reais configuradas na máquina.

## Documentação

- [README.md](./README.md): versão completa em inglês.
- [ARCHITECTURE.md](./ARCHITECTURE.md): arquitetura, responsabilidades dos módulos e fluxo de dados.
- [CONTRIBUTING.md](./CONTRIBUTING.md): guia de contribuição.
- [CHANGELOG.md](./CHANGELOG.md): histórico de versões.
