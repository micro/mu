# Design

This document serves as a guide post for the design of Mu

## Overview

Mu is effectively a new social media platform that attempts to rectify alot of the deficiencies with existing platforms like Twitter, Facebook, YouTube, etc. 
The primary issue is one of usury, explotiation and addiction. American owned corporations are exploiting their users with click/rage bait and various 
advertising tools and algorithms to drive further engagement and profit. There number one goal isn't satisfaction of users, it's to make money. The
majority of these companies are publicly trading, meaning they are beholden to the markets, not those who use the product, but the money managers 
and market makers who drive stock prices up and down. Essentially it all comes back to money and nothing makes more money than addiction.

So ultimately the goal, after many years of frustration, is to build an alternative social platform that does away with these exploits. 
Removing algorithmic feeds, removing click and rage bait, removing ads, removing likes and retweets. Removing all the variety of things 
that keep us glued to the phone and screens as opposed to being an effective tool to better enhance our lives. Their is a toxicity to 
existing social media that needs to be stamped out. Money as a tool can be used to gamble or to pay bills, alcohol can be used as 
a disinfectant or to poison and intoxicate the body. Social media and technology in general suffer similar consequences of these 
addictive properties. By removing the ability to use it for self harm, we can start to use it for good. We see some of that 
already but not without significantly more harm done to society.

What does a social platform look like without the addiction? A fixed feed with relevant content from you and those around you, 
not a follow model, not a user or like model, but one that focuses on your thoughts and reflection and then others near you. 
There's no following significant public figures, there's no game to try get as many followers as possible, its about going 
back to what microblogging was about. And to really focus on content being about reflection, not instant commenting. WE
need to discipline ourselves by not reacting to everything we see or think. Not everything needs to be tweeted. Content 
should be posted with deep introspection and so that's the focus. Then we move on to things like useful AI chat with 
something that has sound ethics and morals baked in e.g [Fanar](https://fanar.qa/en) which uses Islamic and eastern 
cultural values. WE don't need yet another US trained LLM that focuses on western english content and an arguably
corrupt moral standard. Beyond that we bring news and video into the mix as well. The latest headlines from around 
the globe and videos from select channels on YouTube. No short videos, no click bait, no ads, just useful content. 

## Future Work

Part of the goal in building something like this will require including an economic system or marketplace. This 
is not a day one concern but over time we can see that as you build societal fabric part of that is monetary
transaction. [Base](https://www.base.org/) a new app from Coinbase is a good example of this. While some of the 
usecases are not ideal e.g still appealing to a cypherpunk/nft generation, the foundations of the app and broader 
story are sound. Essentially its one of removing the middlemen and establishing a new standard for social connection.

## Architecture

Everything is currently written in Go and will continue to do so. Unfortunately switching between different frameworks
and languages for frontend and backend are not ideal. With one language and one monolithic app we have a better opportunity
to create something lasting that can be iteratively evolved. Go has showcased this time and time again and in my own work too.

## Building Blocks

Some of the app building blocks to think about are Chat, News, Video, Posts, Wallet, etc. It's a non exhaustive list and we won't 
introduce everything on day 1. The app also needs a way to save data, allow people to create accounts and authenticate. And we most
definitely need an API that provides broader use not just for the app but to build other tools on. At the moment the app has a very 
basic API with data being saves to files locally on disk. It's single process and runs on a single server which is fine for the long 
term. There is no need to distribute the architecture of software in a way that's going to need to scale out from that. In all likelihood 
we will enable to run copies of Mu themselves and contribute to the network rather than attempting to build any one centralised host. This 
will not be like Mastodon and will not use ActivityPub, if we need a protocol then we will define something simple using JSON called the MUCP 
protocol (Micro Communication Protocol). ActivityPub while useful also has its quirks and the goal of that community is very different. It also 
does not address payments, trade, etc and that's something we want to address. We want to think about including backend services that can be used 
via chat or in the app itself. Which leads to the next thing to think about. At a certain point a marketplace does include the need for services 
and we don't need these to be full apps that are visually based. With everything moving to an AI based experience, Chat becomes the main point of 
dialog and agents are what's used in the background to fulfill the work. Agents will require access to services to fulfill the needs of the user. 

## Agents

More on this soon.
