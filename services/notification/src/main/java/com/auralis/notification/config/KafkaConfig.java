package com.auralis.notification.config;

import com.auralis.notification.events.InvalidEnvelopeException;
import org.apache.kafka.common.TopicPartition;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.kafka.listener.DeadLetterPublishingRecoverer;
import org.springframework.kafka.listener.DefaultErrorHandler;
import org.springframework.util.backoff.ExponentialBackOff;

@Configuration
public class KafkaConfig {

    /**
     * Same policy as kafkax.Consumer: up to 5 attempts with exponential backoff
     * from 500ms capped at 30s, then the record goes to "<topic>.dlq" with the
     * failure in its headers and the offset moves on. An undecodable event is
     * dead-lettered on the first failure since retrying cannot fix it.
     */
    @Bean
    DefaultErrorHandler kafkaErrorHandler(KafkaTemplate<Object, Object> template) {
        var recoverer = new DeadLetterPublishingRecoverer(template,
                (record, ex) -> new TopicPartition(record.topic() + ".dlq", -1));
        var backoff = new ExponentialBackOff(500, 2.0);
        backoff.setMaxInterval(30_000);
        backoff.setMaxAttempts(4);
        var handler = new DefaultErrorHandler(recoverer, backoff);
        handler.addNotRetryableExceptions(InvalidEnvelopeException.class);
        return handler;
    }
}
