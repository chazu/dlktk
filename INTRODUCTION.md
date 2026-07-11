# An Introduction to dlktk

*A gentle, from-zero guide to the ideas behind dlktk — defeasible reasoning, structured argument, and automated inference — and how the tool turns them into something you can drive from a terminal.*

You do not need any background in logic, philosophy, or AI to read this. If you can follow a pros-and-cons list, you can follow everything here. By the end you will understand **what the tool computes for you, why, and how to read its answers.**

---

## 1. The problem it solves

Teams make decisions, write them down, and then forget *why*. Six months later someone asks "why did we pick the mutex?" and the reasoning is gone — scattered across chat threads, a doc nobody updated, and people's memories.

The usual tool is a **pros-and-cons list**. It has two fatal weaknesses:

1. **It can't represent disagreement about the cons.** Someone objects "RWLocks starve writers." Someone else replies "not for *our* workload." A flat list has nowhere to put that reply — it just becomes another bullet, and the reader has to re-derive who's right.
2. **It doesn't compute anything.** It's a static record. It can't tell you *what currently stands* after the latest objection, or which questions are still genuinely open.

dlktk fixes both. It records a discussion as a **graph of arguments**, and then it **runs an inference algorithm** over that graph to tell you, at any moment, which positions are currently justified, which are defeated, and which are still contested. When the argument changes, the answer changes automatically — and you can always ask *why*.

---

## 2. The concepts, built from nothing

### 2.1 Defeasible reasoning

Classical logic is **monotonic**: once you prove something, adding more facts never takes it away. Real reasoning isn't like that. "Tweety is a bird, so Tweety flies" — until you learn Tweety is a penguin. The new fact *defeats* the old conclusion.

Reasoning where conclusions can be **withdrawn in light of new information** is called **defeasible**. This is the natural logic of design debates: a position looks good *until* someone raises an objection, which holds *until* someone rebuts it, and so on. Conclusions are provisional. dlktk is built to track exactly this kind of shifting, provisional justification.

### 2.2 IBIS: a vocabulary for disagreement

Before we can reason about an argument, we need to write it down in a structured way. dlktk uses **IBIS** (Issue-Based Information System), a decades-old scheme with just three node types:

- **Issue** — a question to resolve. *"Which lock should the cache use?"*
- **Position** — a candidate answer to an issue. *"Use a mutex."* / *"Use an RWLock."*
- **Argument** — a reason that bears on a position or on another argument. *"RWLocks can starve writers."*

And three ways to link them:

- **responds_to** — a position responds to an issue.
- **objects_to** — an argument attacks a position or another argument.
- **supports** — an argument backs a position or another argument.

That's the whole capture vocabulary. Notice the key move: **an argument can object to another argument.** That's what a flat list can't do, and it's where the power comes from (see §2.6).

### 2.3 Turning argument into a graph of attacks

Here is the conceptual leap. Strip away the words and keep only one relation: **who attacks whom.**

- An `objects_to` link is an **attack**.
- If an issue is **select_one** (the positions are mutually exclusive — you can pick only one), then its positions **attack each other** automatically. Choosing one rules out the others.

So a discussion becomes a directed graph: nodes are positions and arguments, edges are attacks. This object has a name in the AI literature: an **abstract argumentation framework**, introduced by Phan Minh Dung in 1995. Everything dlktk computes is defined over this attack graph.

### 2.4 What "standing" means: the grounded labelling

Given a graph of attacks, which arguments *survive*? Dung's answer assigns every node one of three labels:

- **IN** — *justified.* It stands. Every attacker of it is defeated.
- **OUT** — *defeated.* At least one of its attackers is IN.
- **UNDEC** — *undecided.* Genuinely contested — it's in an unresolved standoff (a cycle or a tie).

These aren't assigned by vote or by hand. They're computed by a simple rule applied over and over until nothing changes (a **fixpoint**):

> **Round 1.** Any node with *no* attackers is vacuously justified → **IN**.
> **Round 2.** Any node attacked by something now IN is → **OUT**.
> **Round 3.** Any node whose attackers are *all* now OUT becomes → **IN**.
> Repeat until no labels change. Whatever is left stuck is → **UNDEC**.

This particular labelling is called the **grounded** extension. It is the *most skeptical* consistent position: it only calls something justified when it's forced to. It's also **unique** — there's exactly one grounded labelling for any graph, so the answer is deterministic and never depends on argument order. This is the automated reasoning at the heart of dlktk. You build the graph; the fixpoint tells you what stands.

### 2.5 Reinstatement: why objecting to an objection matters

Watch what the fixpoint does with a chain of three:

```
position  ←objects_to←  argument A  ←objects_to←  argument B
```

- B has no attacker → **IN**.
- A is attacked by B (IN) → **OUT**.
- The position's only attacker, A, is now OUT → the position is **IN** again.

B **reinstated** the position by defeating its attacker. This is the thing a pros-and-cons list fundamentally cannot express — the rebuttal-to-the-objection — and it's why argument structure carries information a flat tally throws away. dlktk marks reinstated nodes explicitly (the `↩` glyph).

### 2.6 Preference: turning a tie into a decision

Two positions that only attack each other (classic select_one rivals) form a symmetric standoff. The fixpoint can't break it — both end up **UNDEC**. That's correct: *on the argument alone, it's a genuine tie.*

To decide, you need to add information: a **preference**. `prefer(RWLock, mutex, basis=throughput)` says "for the throughput reason, RWLock beats mutex." dlktk turns this into an asymmetry:

> An attack is a **defeat** only if it survives preference. If you prefer B over A, then A's attack on B is *neutralized* — it no longer counts.

Now the graph is asymmetric, the fixpoint runs to completion, and one position comes out IN. **Defeat = attack that survives preference** is the single equation that makes the whole system *defeasible* in the technical sense: preferences let justified conclusions be overturned cleanly.

Preferences are transitively closed (if A≻B and B≻C then A≻C), with cyclic preferences rejected when asserted — see the design doc for why.

### 2.7 Cycles and stalemates

Some graphs never settle. Three positions in a select_one issue all attack each other; an odd-length objection cycle (A attacks B attacks C attacks A) loops forever. The fixpoint correctly leaves everything **UNDEC**. dlktk recognizes this specific shape — *every position UNDEC, none actually defeated* — and tells you plainly: **this is a stalemate; a preference will resolve it, but another argument on the same nodes won't.** That stops you (or an agent) from flailing by piling on more objections that just re-enter the loop.

---

## 3. How dlktk implements all this

### 3.1 One verb per concept

Every idea above maps to a command:

| You want to… | Concept | Command |
|---|---|---|
| Pose a question | Issue | `dlktk raise "…" [--card select_one\|open]` |
| Offer an answer | Position | `dlktk propose <issue> "…"` |
| Object to something | Attack | `dlktk object <target> "…"` |
| Back something | Support | `dlktk support <target> "…"` |
| Break a tie | Preference | `dlktk prefer <winner> <loser> --basis …` |
| Record the call | Decision | `dlktk decide <issue> <position> --basis …` |
| Overturn the call | Supersession | `dlktk supersede <issue> <position> --basis …` |
| See what stands | Grounded labelling | `dlktk status [issue]` |
| See the shape | The graph | `dlktk tree [issue]` |
| Understand one label | Local explanation | `dlktk why <node>` |
| Follow the whole derivation | The fixpoint, traced | `dlktk explain <issue>` |
| See open questions | The UNDEC set | `dlktk agenda` |
| Ask "what next?" | Legal useful moves | `dlktk moves <issue>` |

The capture verbs (`raise`/`propose`/`object`/…) build the graph. The read verbs (`status`/`tree`/`why`/`explain`/…) run and report the inference. You never label anything by hand — you state arguments, and the labelling is derived.

### 3.2 Everything is an append-only fact

dlktk stores each move as an immutable **fact** in a bitemporal fact store (pudl). Nodes, links, preferences, decisions — all facts, each stamped with who made it and when. There is **no edit and no delete.** To correct a claim you `retract`/`concede` it (which closes it off without erasing it) and state a new one. This is deliberate: a decision record is only trustworthy if you can see that a claim was *withdrawn*, not silently rewritten. Provenance outranks convenience.

Because the store is **bitemporal**, it remembers both *when something was true in the discussion* and *when it was recorded*. That means you can ask the tool to **replay** the labelling as it stood at any past moment (`dlktk replay <issue> --as-of T`) — the reasoning is reconstructable, not just the current snapshot.

### 3.3 Where the "AI" actually is

There's no neural network here and nothing stochastic. The "automated reasoning" is the grounded fixpoint of §2.4 — about 25 lines of deterministic code that reads the attack graph and labels it. That's a feature: the answer is **explainable by construction.** Every label has a reason you can print, and the same inputs always give the same output.

---

## 4. A worked example, start to finish

Let's decide a real question and watch the reasoning move. (Output is verbatim; ids are auto-generated.)

**Set it up — an issue and two rival positions:**

```
dlktk new "lock choice"
dlktk raise "which lock for the cache?"        # → issue molah-dimut
dlktk propose molah-dimut "mutex"              # → molat-tuval  (alice)
dlktk propose molah-dimut "RWLock"             # → molij-tugut  (bob)
```

Two select_one rivals, mutually attacking → both UNDEC, a tie.

**Carol objects to RWLock:**

```
dlktk object molij-tugut "writers can starve under read load"   # → moliv-zisod (carol)
dlktk status molah-dimut
```
```
  IN    p:molat-tuval  "mutex"  (reinstated)
  OUT   p:molij-tugut  "RWLock"  (defeated by molat-tuval, moliv-zisod)
  → molat-tuval justified
```

The objection defeats RWLock. With its rival gone, **mutex is reinstated and now stands.**

**Bob rebuts the objection** (argues against the argument):

```
dlktk object moliv-zisod "the cache is read-heavy; starvation won't occur in practice"   # → molov-valus (bob)
dlktk status molah-dimut
```
```
  UNDEC p:molat-tuval  "mutex"
  UNDEC p:molij-tugut  "RWLock"
  → mutual stalemate — molat-tuval vs molij-tugut all UNDEC, none defeated;
    a preference resolves this (a new argument helps only if it defeats from outside the stalemate)
```

Bob's rebuttal defeated Carol's objection, which **reinstated RWLock**. Now both positions are back in contention with nothing separating them — the tool detects the **stalemate** and tells you exactly what will break it.

**Alice states a preference:**

```
dlktk prefer molij-tugut molat-tuval --basis throughput
dlktk status molah-dimut
```
```
  OUT   p:molat-tuval  "mutex"  (defeated by molij-tugut)
  IN    p:molij-tugut  "RWLock"  (reinstated)
  → molij-tugut justified
```

The preference neutralizes RWLock's incoming attack, the fixpoint completes, and **RWLock is justified.** The reasoning got there on its own each step — you only ever *stated arguments and one preference.*

---

## 5. Following the reasoning

When you want to know *how* the labelling was reached, ask `explain`:

```
dlktk explain molah-dimut --brief
```
```
1. attacks derived:
   p:molat-tuval ⚔ p:molij-tugut  (select_one, neutralized by preference (basis=throughput))
   p:molij-tugut ⚔ p:molat-tuval  (select_one)
   a:moliv-zisod ⚔ p:molij-tugut  (objection)
   a:molov-valus ⚔ a:moliv-zisod  (objection)
   preferences:
     p:molij-tugut ≻ p:molat-tuval (basis=throughput)

2. automated reasoning — grounded fixpoint:
   round 1:
     IN    a:molov-valus  (no defeaters)
   round 2:
     OUT   a:moliv-zisod  (defeated by a:molov-valus [IN])
   round 3:
     IN    p:molij-tugut  (reinstated — defeater(s) a:moliv-zisod now OUT)
   round 4:
     OUT   p:molat-tuval  (defeated by p:molij-tugut [IN])

3. outcome:
   OUT   p:molat-tuval  "mutex"
   IN    p:molij-tugut  ← justified  "RWLock"
```

This is the whole derivation in three parts: how the attack graph was built (and where the preference *neutralized* an attack), the **round-by-round fixpoint** with a reason for every label, and the result. It's the single best way to see *where the automated reasoning was applied and how it's structured.*

Other lenses on the same computation:

- **`tree`** — the argument as an indented outline, with the standing decision marked (`★`) and a legend. Add `--labels` to overlay IN/OUT/UNDEC.
- **`show <node>`** — one node in full: its text, author, label, and every incident link with the peers' text inlined.
- **`why <node>`** — a *local* explanation: a single node's attackers (with their text), their labels, and the moves that would flip it.
- **`agenda`** — the worklist: every UNDEC node, plus issues whose labelling has settled and only await a `decide`, plus issues with no positions yet.
- **`moves <issue>`** — legal *and useful* next moves with their effects, so you (or an agent) never have to guess what to do next.

---

## 6. Being justified vs being decided

There are two different notions of "settled," and dlktk keeps them apart:

- **Justified (IN)** is *computed* — the argument structure currently supports this position.
- **Decided** is *recorded* — a human (or agent) ran `dlktk decide` to commit to a position.

Usually you decide *in favour of* the justified position, and `status`/`tree` show a `✓ decided` marker confirming the match. But you can also **override** — decide for a position the argument doesn't currently justify (sometimes the call is political, or time-boxed). dlktk records that the decision was an override rather than hiding the mismatch. The point throughout: the tool never pretends the reasoning says something it doesn't.

---

## 7. For agents

Every verb is equally drivable by a program. Add `--format json` to any read for a structured envelope, and run `dlktk discover` to get the full machine-readable contract — the move/read vocabulary, each move's legality precondition, the JSON shape every read returns, the exit-code/error catalog, and the global flags. An agent can learn to drive the tool *cold* from that one document, then use `moves`/`agenda`/`why`/`explain` to reason about what to do — the same surfaces a human uses, in JSON.

---

## Glossary

- **Defeasible** — a conclusion that can be withdrawn when new information arrives.
- **IBIS** — issue / position / argument, the capture vocabulary.
- **Attack** — a directed "this defeats that" edge, from an objection or a select_one rivalry.
- **Argumentation framework** — the attack graph (Dung 1995).
- **Grounded labelling** — the unique, most-skeptical assignment of IN/OUT/UNDEC computed by the fixpoint.
- **IN / OUT / UNDEC** — justified / defeated / genuinely contested.
- **Reinstatement** — a node becomes IN again because its attacker was itself defeated.
- **Preference** — `prefer(winner, loser, basis)`; turns a tied attack into a one-way defeat.
- **Defeat** — an attack that survives preference (`attack(a,b)` unless `prefer(b,a)`).
- **Stalemate** — all positions UNDEC with none defeated; a cycle or symmetric tie needing a preference.
- **Bitemporal** — the store tracks both when a fact was true and when it was recorded, enabling replay.
- **Decision** — a recorded commitment to a position, distinct from it being justified.

## Where to go next

- [`README.md`](README.md) — install, quick example, command summary.
- [`wicked-problems.md`](wicked-problems.md) — why adjudication alone isn't enough for open-ended ("wicked") questions, and the divergence/exploration features built for them: untested-winner surfacing, `reframe`/`synthesize`/`assume`, `whatif`/`crux`/`worlds`, value audiences, review horizons.
- [`dlktk-design.md`](dlktk-design.md) — the full design, including the grounded-semantics implementation (§4) and the resolved design questions (§16), several of which were *themselves* decided by running the dialectic through dlktk.
- [`examples/`](examples/) — real, replayable discussions (`dlktk import examples/q2-preference-transitivity.ndjson`), each a worked argument with a recorded outcome.
- Phan Minh Dung, *"On the acceptability of arguments…"* (1995) — the original paper behind the labelling, if you want the theory.
