# VPC Proof Agent

Um **agente de diagnóstico e coleta de evidências** escrito em Go, que valida e comprova, tecnicamente, que um ambiente de rede AWS provisionado manualmente está funcionando corretamente.

O agente executa em uma instância EC2 (Amazon Linux 2) e valida um ambiente-alvo composto por uma VPC (`10.0.0.0/16`), uma sub-rede pública (`10.0.1.0/24` com atribuição automática de IP público), um Internet Gateway, uma tabela de rotas com rota padrão para o IGW, um Security Group e uma instância EC2 (`t2.micro`).

> **O agente NÃO provisiona recursos AWS.** Ele parte do princípio de que o ambiente existe e o valida, gerando evidências reproduzíveis.

- **CLI** — administração, diagnósticos aprofundados e geração de relatórios, usado localmente via SSH.
- **REST API (v1)** — exposta publicamente para comprovar acessibilidade pela internet, roteamento externo e configuração do Security Group.

---

## Sumário

- [Como o agente valida o laboratório](#como-o-agente-valida-o-laboratório)
- [Mapeamento dos requisitos do laboratório](#mapeamento-dos-requisitos-do-laboratório)
- [Visão técnica](#visão-técnica)
- [Como executar localmente](#como-executar-localmente)
- [Segurança](#segurança)
- [Documentação](#documentação)

---

## Como o agente valida o laboratório

O objetivo central é comprovar, com evidências técnicas, que o ambiente de rede AWS foi configurado corretamente. O agente:

1. **Extrai os metadados da instância** via IMDSv2 (ID da instância, IP privado, IP público e zona de disponibilidade).
2. **Executa sondas de rede** para verificar se o IP privado pertence ao CIDR da VPC/sub-rede corretos, se existe rota padrão e gateway, se a resolução de DNS funciona e se há conectividade HTTPS de saída.
3. **Compara o IP público** reportado pela AWS com o IP público observado por um serviço de eco externo.
4. **Traduz falhas** em dicas acionáveis de troubleshooting AWS (por exemplo, "Verifique se o IGW está anexado" ou "Verifique a associação da tabela de rotas").
5. **Gera relatórios** de evidências em JSON, Markdown e texto puro.

O agente funciona por duas interfaces complementares:

- **CLI**: utilizada por um avaliador/administrador via SSH para diagnósticos profundos e geração de relatórios.
- **REST API**: exposta publicamente (porta 8080) para provar que a instância é alcançável pela internet, que o roteamento externo funciona e que o Security Group está liberando o tráfego corretamente.

## Mapeamento dos requisitos do laboratório

| Requisito do laboratório | Como o agente comprova |
| --- | --- |
| VPC `10.0.0.0/16` | Validação de pertencimento do IP privado ao CIDR da VPC |
| Sub-rede pública `10.0.1.0/24` com IP público automático | Verificação do CIDR da sub-rede e presença de IP público |
| Internet Gateway (IGW) anexado | Detecção de rota padrão `0.0.0.0/0` via gateway de internet |
| Tabela de rotas com rota padrão para o IGW | Sonda de rota padrão e gateway padrão ativo |
| Security Group (SSH/22 e HTTP/8080) | Teste de conectividade externa de entrada via API pública e validação das regras |
| Instância EC2 `t2.micro` | Extração de metadados via IMDSv2 (ID da instância, AZ) |
| Acessibilidade pela internet | Comparação do IP público AWS com o IP público externo |

## Visão técnica

- **Linguagem:** Go 1.26
- **Arquitetura:** Clean Architecture com o layout padrão de projetos Go (`cmd/`, `internal/`, `pkg/`).
- **Mock local da AWS:** LocalStack (acessado via CLI `lstk` ou `aws` com endpoints customizados).
- **Provisionamento do laboratório:** script idempotente em `scripts/setup-localstack.sh`, que cria VPC, sub-rede, IGW, tabela de rotas e Security Group dentro do LocalStack usando a AWS CLI.
- **Observabilidade:** logging estruturado (JSON/Texto), IDs de requisição e métricas no formato Prometheus.

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
```

Para remover o laboratório provisionado:

```bash
make localstack-teardown
```

## Segurança

- O agente **nunca** provisiona recursos e **nunca** lê ou expõe credenciais AWS.
- A REST API é protegida por autenticação baseada em token e por limitador de requisições.
- Os scripts do LocalStack **forçam credenciais fictícias** (`AWS_ACCESS_KEY_ID=test`, `AWS_SECRET_ACCESS_KEY=test`) e um endpoint dedicado, garantindo que nunca afetem uma conta AWS real — mesmo que o avaliador possua credenciais reais configuradas na máquina.

## Documentação

- [README.md](./README.md) — versão completa em inglês.
- [ARCHITECTURE.md](./ARCHITECTURE.md) — arquitetura, responsabilidades dos módulos e fluxo de dados.
- [CONTRIBUTING.md](./CONTRIBUTING.md) — guia de contribuição.
