# Desafio Stress Test

Desafio da pós de Go. É uma CLI pra fazer teste de carga em serviço web: você
passa a URL, quantas requisições quer fazer e quantas rodam ao mesmo tempo, e no
final ela mostra um relatório com o tempo total e os status HTTP que voltaram.

## Rodando com Docker

O jeito pedido no enunciado. Primeiro gera a imagem:

```
docker build -t stresstest .
```

E depois roda passando os parâmetros direto no `docker run`:

```
docker run stresstest --url=http://google.com --requests=1000 --concurrency=10
```

Se quiser testar algo que está rodando na sua própria máquina, lembra que
`localhost` dentro do container é o próprio container. No Docker Desktop dá pra
usar `host.docker.internal`:

```
docker run stresstest --url=http://host.docker.internal:8080 --requests=500 --concurrency=20
```

No Linux pode ser necessário rodar com `--network=host` e aí usar `localhost`
normalmente.

## Rodando sem Docker

Precisa do Go 1.22 ou mais novo.

```
go run . --url=http://google.com --requests=100 --concurrency=10
```

Ou gerando o binário:

```
go build -o stresstest .
./stresstest --url=http://google.com --requests=100 --concurrency=10
```

## Parâmetros

| Flag              | Atalho | Padrão | Descrição                              |
|-------------------|--------|--------|----------------------------------------|
| `--url`           | `-u`   |        | URL do serviço (obrigatória)           |
| `--requests`      | `-r`   |        | Total de requisições (obrigatória)     |
| `--concurrency`   | `-c`   | 1      | Quantas requisições rodam em paralelo  |
| `--timeout`       |        | 30s    | Tempo limite de cada requisição        |

O `--timeout` não estava no enunciado, coloquei porque sem ele uma URL que
trava segura o teste inteiro. Aceita o formato de duração do Go (`500ms`, `2s`,
`1m`).

A URL precisa ter o `http://` ou `https://` na frente. Se a concorrência for
maior que o número de requisições, ele só sobe a quantidade de workers que
precisa.

## Exemplo de saída

```
Testando http://google.com com 50 requests (10 simultâneas)...

================ RELATÓRIO ================
Tempo total.............: 4.292s
Requests realizados.....: 50
Requests com status 200.: 50
Requests por segundo....: 11.65
Latência (min/méd/máx)..: 751ms / 811ms / 941ms

Distribuição dos status HTTP:
  [200] OK                     50
===========================================
```

Quando volta outros códigos eles aparecem um embaixo do outro, ordenados:

```
Distribuição dos status HTTP:
  [200] OK                     20
  [404] Not Found              10
  [500] Internal Server Error  10
```

E se alguma requisição nem chegar a ter resposta (timeout, conexão recusada,
DNS que não resolve), ela entra numa seção separada, porque não tem status HTTP
pra contar:

```
Falhas sem resposta HTTP:
  timeout                        4
```

Essas falhas também contam no total de requests realizados, já que a
requisição foi feita, só não voltou nada.

Uma observação sobre o `http://google.com`: ele responde com um redirect pra
`www.google.com`, e o cliente HTTP do Go segue o redirect sozinho. Por isso o
relatório mostra 200, que é o status da resposta final.

Em volume alto, o Google pode limitar as requisições. Nesse caso, o relatório
pode mostrar `429` ou falhas como `stopped after 10 redirects`, e a quantidade
de respostas 200 varia entre as execuções. Para validar as contagens, use um
serviço próprio.

Apertar Ctrl+C no meio (ou dar `docker stop`) interrompe o teste, mas o
relatório ainda é impresso com o que deu tempo de rodar.

## Como funciona

O código principal está em `internal/loadtest/runner.go` e segue a ideia de
worker pool que vimos na aula de multithreading:

- sobe `concurrency` goroutines (os workers), todas lendo do mesmo channel
  `jobs`;
- uma goroutine produtora coloca exatamente `requests` itens nesse channel e
  depois fecha ele;
- cada worker pega um item, faz o GET e manda o resultado (status, tempo, erro)
  num channel `results`;
- quando o channel `jobs` fecha os workers saem do `range`, o `WaitGroup` chega
  a zero e o `results` é fechado também;
- a função `Run` vai lendo o `results` e montando o relatório.

Como é o produtor quem controla quantos itens entram no channel, não importa
se a divisão entre os workers é exata (1000 / 7 por exemplo). Quem estiver livre
pega o próximo, e o total sempre bate.

Só uma goroutine mexe no relatório (a que lê o `results`), então não precisei de
mutex pros contadores.

Alguns detalhes:

- o corpo da resposta é lido até o fim e fechado. Sem isso o Go não reaproveita
  a conexão e cada requisição abriria um socket novo, o que distorce o teste;
- o `http.Transport` é configurado com `MaxIdleConnsPerHost` igual à
  concorrência. O padrão do Go é 2, então com 10 workers ia ficar abrindo e
  fechando conexão o tempo todo;
- o contexto vem do `signal.NotifyContext`, e as requisições são criadas com
  `http.NewRequestWithContext`, então o Ctrl+C cancela até as que estão em
  andamento.

A parte de CLI usa o Cobra, igual na aula de CLI. Como é um comando só, as flags
ficam direto no comando raiz em `cmd/root.go`.

## Testes

```
go test ./...
```

Os testes ficam em `internal/loadtest/loadtest_test.go` e usam `httptest`, então
não precisam de internet. Eles conferem:

- se o número de requisições que chegam no servidor é exatamente o pedido,
  inclusive quando a divisão pelos workers não é exata ou quando a concorrência
  é maior que o total;
- se nunca passa de `concurrency` requisições em paralelo;
- se a contagem de 200, 404 e 500 bate;
- se timeout e conexão recusada caem na seção de falhas;
- se o cancelamento do contexto para o teste no meio.

Com o gcc instalado dá pra rodar com o detector de race também:

```
go test -race ./...
```

## Estrutura

```
main.go                         só chama o cmd.Execute()
cmd/root.go                     flags, validação e chamada do teste
internal/loadtest/runner.go     worker pool e as requisições
internal/loadtest/report.go     contagem dos resultados e impressão do relatório
internal/loadtest/loadtest_test.go
Dockerfile
```

O Dockerfile é multi-stage: compila com a imagem do Go e copia só o binário pra
uma imagem `scratch`, que nem a da aula de deploy. A diferença é que precisei
copiar os certificados CA também, senão qualquer URL `https` dá erro de
certificado, já que a `scratch` vem vazia.
