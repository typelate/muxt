# Explore SSE

I don't want to use any third party SSE libraries.

The template signature should allow the template author to send `lastEventID`. The value would come from 
the HTTP request header "Last-Event-Id". This should have the same semantics as the other identifiers like (response,
request, ctx...). In particular, if a user has `GET /{lastEventID}` set the path parameter is preferred over the
header based identifier.

The template name also needs to specify what the send callback should look like (and what the method will send to it).
There are four things that a message can have event, id, retry, and data. I'd like 

The new generated handler for SSE endpoints should start with this:
