package com.auralis.notification.web;

import org.springframework.http.HttpStatus;

/**
 * An error with a platform error code. Rendered as
 * {"error":{"code":..,"message":..,"request_id":..}}, the same body every Go
 * service writes through errcodes.Write.
 */
public class ApiException extends RuntimeException {

    private final HttpStatus status;
    private final String code;

    public ApiException(HttpStatus status, String code, String message) {
        super(message);
        this.status = status;
        this.code = code;
    }

    public HttpStatus status() {
        return status;
    }

    public String code() {
        return code;
    }

    public static ApiException unauthorized(String msg) {
        return new ApiException(HttpStatus.UNAUTHORIZED, "auth.unauthorized", msg);
    }

    public static ApiException badRequest(String msg) {
        return new ApiException(HttpStatus.BAD_REQUEST, "request.validation_failed", msg);
    }

    public static ApiException notFound(String msg) {
        return new ApiException(HttpStatus.NOT_FOUND, "request.not_found", msg);
    }
}
