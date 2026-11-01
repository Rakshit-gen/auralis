package com.auralis.notification.events;

/** A message that can never be processed. It goes to the DLQ without retries. */
public class InvalidEnvelopeException extends RuntimeException {

    public InvalidEnvelopeException(String message) {
        super(message);
    }
}
