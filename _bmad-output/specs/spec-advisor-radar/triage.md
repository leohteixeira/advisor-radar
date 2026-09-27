# Triage classifier

Copy this behavior from `radar-triage` into the `triage` service. Resilience in `architecture.md` wraps it; it does not replace the port, the questions, or the heuristic.

## Port

`Message`: `ID`, `Channel`, `Text`, `Previous` (optional recent texts from the same customer).

Model state is only `canal`, `mensagem`, and `mensagens_anteriores` when `Previous` is non-empty. Numbers and dates stay out of that state.

`Result`: `Intent`, `IntentProb`, `Frustration` (0 calm through 3 very frustrated, possibly fractional from the model), `ChurnRisk`, `WantsHuman`, `Classifier` (`jev` or `heuristic`), `ModelVersion`, `Degraded`, `Latency`, `CostUSD`.

Wire intents: `operacional`, `cambio`, `tributacao`, `investimento`, `resgate`, `reclamacao`, `encerramento`, `contato`.

## Jev

`POST {base}/v1/evaluate`. Default base `https://ai-gateway.vercel.sh`. Default model `typesafe-ai/jev`. Header `Authorization: Bearer` plus the gateway key (`AI_GATEWAY_API_KEY`). Body limit on the response read is 1 MiB.

Always send `providerOptions.gateway.zeroDataRetention: true`.

The prototype client retries 429 and 5xx internally (3 attempts, 150 ms base backoff, 5 s HTTP timeout) and does not read `Retry-After`. The service does not keep that inner retry. One HTTP attempt goes through the timeout layer, which is 2 seconds. Retry, `Retry-After`, the limiter, the breaker, and the bulkhead are the stack in `architecture.md`.

Questions are atomic and combined in code. Copy the instructions and criteria verbatim.

Intent choice. Instructions: "Qual é o assunto principal da mensagem deste cliente de uma corretora de investimentos internacionais?"

| Choice | Criterion |
|---|---|
| operacional | acesso ao app, senha, cadastro, documentos ou atualização de dados |
| cambio | envio ou recebimento de dinheiro do Brasil, câmbio, remessa ou wire |
| tributacao | imposto de renda, DARF, informe de rendimentos ou declaração |
| investimento | dúvida sobre ações, ETFs, renda fixa, fundos ou composição da carteira |
| resgate | sacar, resgatar ou transferir saldo da conta |
| reclamacao | insatisfação com o atendimento, uma cobrança, um erro ou demora |
| encerramento | pedido explícito para encerrar ou fechar a conta |
| contato | pede retorno, ligação ou contato do assessor sem dizer o assunto |

Frustration score, low to high. Instructions: "Qual o nível de frustração demonstrado pelo cliente nesta mensagem?"

1. calmo: tom neutro ou cordial
2. incomodado: demonstra impaciência leve
3. frustrado: reclama de forma clara ou cita tentativas anteriores sem sucesso
4. muito frustrado: tom agressivo, ameaças ou indignação forte

Churn boolean. Instructions: "O cliente menciona sair da empresa, levar o dinheiro para outra instituição ou encerrar a conta?" True: "cita explicitamente sair, trocar de corretora, transferir tudo para outro lugar ou encerrar a conta". False: "não menciona deixar a empresa".

Human boolean. Instructions: "O cliente pede para falar com uma pessoa, com o assessor ou com um atendente humano?" True: "pede explicitamente atendimento humano, ligação ou o assessor". False: "não faz esse pedido".

Missing intent choice is an error from the Jev adapter, which `Fallback` then degrades. Map `answers.intent.choice` and `probabilities[choice]` to intent and intent probability. Map frustration `score`, and churn and human `probability`. Gateway cost is `providerMetadata.gateway.cost` when present. Do not require `confidence`.

## Heuristic

Accent-fold and lowercase the text, then match substrings. First matching intent wins.

| Intent | Substrings |
|---|---|
| encerramento | encerrar, fechar minha conta, cancelar a conta, encerramento |
| reclamacao | absurdo, pessimo, reclamacao, reclame aqui, descaso, cobrado duas, cobranca indevida, ninguem resolve |
| tributacao | imposto, darf, informe de rendimentos, declaracao, receita federal, ` ir ` |
| cambio | remessa, cambio, wire, enviar dinheiro, mandar dinheiro, transferencia internacional, spread |
| resgate | sacar, saque, resgatar, resgate, retirar |
| investimento | acao, acoes, etf, carteira, renda fixa, treasury, fundo, dividendo, reit |
| operacional | senha, login, acesso, aplicativo, app, cadastro, documento, token |
| contato | me ligar, me liga, retorno, entrar em contato |

No match: `operacional` at 0.3. A match: that intent at 0.6. Churn substrings set churn to 0.8: outra corretora, outro banco, vou sair, levar meu dinheiro, tirar tudo, encerrar, concorrente. Human substrings set human to 0.8: falar com alguem, falar com uma pessoa, atendente, humano, meu assessor, me liga, ligacao. Frustration is the count of these substrings, capped at 3: absurdo, pessimo, vergonha, de novo, ninguem, cansado, `!!`, descaso, ate agora. Classifier `heuristic`, version `keywords-v1`. The heuristic does not return an error.

## Fallback

Timeout wraps only the primary call. If the caller context is already done, return that error and do not call the heuristic. Otherwise any primary error calls `OnDegrade` and then the heuristic. Set `Degraded` on that result. If the heuristic also fails, join both errors.

The prototype `NeedsReview` is `Degraded || IntentProb < threshold`, and its test uses 0.7. The product queue does not copy the degraded clause. Review is `IntentProb < 0.85`. Heuristic scores are 0.3 or 0.6, so a fallback result still enters the queue. The degraded flag only drives "Classificação simplificada".

## Probe

Copy `testdata/messages.json` with the package. The probe runs Jev and the heuristic on that file and prints per-message intent, probability, frustration, churn, human, heuristic intent, and latency, plus accuracy, p50, p95, and cost. Boolean fields count as true at 0.5. A value already in the environment wins over a `.env` file. Baseline after the label correction: Jev 16/16 intent, 16/16 churn, 16/16 human; the copied heuristic was 11/16 intent and is extended to 16/16 on the labeled set; p50 about 290 ms.
