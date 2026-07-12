# Agent Instructions - Exercise for the 'wicked problems' features of dlktk

This document provides a prompt containing a sample 'wicked problem'. Your task is to use the `dlktk` tool - and specifically the features recently added for tackling 'wicked problems' - to try and synthesize possible solutions. Ideally, the system should produce multiple non-trivial insights when the problem is run properly. While it is okay if a single synthesis is reached when the problem is run, the expectation is that the problem space will be thoroughly explored and divergent approaches will be explored; if this does not happen, it likely indicates that the features recently added are insufficiently clear to agents in their application or are not properly implemented.

The next section contains the prompt which is to be used for the issue

## The problem

Although general consensus is that agentic coding has enlarged the capabilities of what an individual programmer or software engineer is able to accomplish per unit time, it has also transformed 'programming' - or more generally the hands-on building of software by an individual - in some troubling ways. Firstly, it has shifted the focus away from creative problem-solving towards the management of agent output - design documents, plans, and code. The volume of code produced via the standard agentic coding loop (e.g. claude code or codex use) is so large as to almost invariably lead to the human programmer no longer being familiar with the layout and functioning of the code. A knock-on effect of this is that the human programmer obtains and retains far less domain-specific knowledge - of the programming technologies used to implement the solution and of the solution itself.
There is growing evidence that this approach leads to skill decay in the human programmer. Whats more, software created in this fashion tends to suffer from accelerated 'software aging' (as defined by David Parnas).
Taking as a given that agentic coding is 'here to stay', how can the actual process of agentic coding be changed to avoid these issues?
