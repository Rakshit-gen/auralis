package com.auralis.notification.web;

import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.Map;

/** The platform error envelope. */
public record ErrorBody(Detail error) {

    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public record Detail(String code, String message, String request_id, Map<String, String> fields) {}

    public static ErrorBody of(String code, String message, String requestId) {
        return new ErrorBody(new Detail(code, message, requestId, null));
    }
}
