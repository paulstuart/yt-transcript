# The point of this whole thing

The overarching goal for this project is to gather lessons and information shared in video format to be condensed to text, summarized and gathered, as a kind of meta TL;DR;

There are many channels with a wealth of information on one simply doesn't have the time to watch them all. So instead:

* Identify all the videos of each channel of interest
* Gather their transcripts for what they have to say
* Condense them individually for concise readability
* Index the whole of the text and summaries to provide high level view into all the information from all the channels of interest -- basically a personal assistant for managing information of interest

## Approach

We want to follow the unix philosophy when best able, so that each unit of functionality used is as simple, robust and easy to reason about as possible. Taking that philosophy forward is how one can compose multiple small single focused command line tools into a powerful data workflow

This repo is only about capturing the transcript from a specific YT video. More tools needed:

* A "channel vaccum" that monitors all videos of a channel and ensures that each them is captured into the transcript store.
* A transcript digester: stores all transcripts with associated metadata, summarizes them, generates keywords for all of them, and then gathers them all into a database for further analysis (likely sqlite w/ FTS and vector embedding)
* A UI for the digester that allows browsing and interacting with the results
* A meta agent that analyzes all of the captured info and analysis and acts as the personal assistant to the user to share with the user only the inforation they want and should be paying attention to.

NOTE: all of this is basically the goal for the github.com/paulstuart/healthweb project -- but it was too unwieldy and we want to try again following the strategy laid out here.