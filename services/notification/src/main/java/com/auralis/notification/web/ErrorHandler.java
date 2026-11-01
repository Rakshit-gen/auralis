package com.auralis.notification.web;

import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.method.annotation.MethodArgumentTypeMismatchException;
import org.springframework.web.servlet.resource.NoResourceFoundException;

/** Maps every failure to the platform error envelope. */
@RestControllerAdvice
public class ErrorHandler {

    private static final Logger log = LoggerFactory.getLogger(ErrorHandler.class);

    @ExceptionHandler(ApiException.class)
    ResponseEntity<ErrorBody> api(ApiException e, HttpServletRequest req) {
        return ResponseEntity.status(e.status()).body(ErrorBody.of(e.code(), e.getMessage(), requestId(req)));
    }

    // A path id that is not a UUID is the caller's mistake, not a 500.
    @ExceptionHandler(MethodArgumentTypeMismatchException.class)
    ResponseEntity<ErrorBody> badParam(MethodArgumentTypeMismatchException e, HttpServletRequest req) {
        return api(ApiException.badRequest("invalid " + e.getName()), req);
    }

    @ExceptionHandler(NoResourceFoundException.class)
    ResponseEntity<ErrorBody> noRoute(HttpServletRequest req) {
        return api(ApiException.notFound("no route for this path"), req);
    }

    @ExceptionHandler(Exception.class)
    ResponseEntity<ErrorBody> unexpected(Exception e, HttpServletRequest req) {
        log.error("unhandled error on {} {}", req.getMethod(), req.getRequestURI(), e);
        return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                .body(ErrorBody.of("internal.unexpected", "an unexpected error occurred", requestId(req)));
    }

    static String requestId(HttpServletRequest req) {
        return req.getHeader("X-Auralis-Request-Id");
    }
}
