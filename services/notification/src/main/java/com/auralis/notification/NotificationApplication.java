// Package com.auralis.notification is the Auralis notification service. It
// keeps a projection of who follows which show and, when a new episode is
// published, writes an in-app notification for every follower.
package com.auralis.notification;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class NotificationApplication {

    public static void main(String[] args) {
        SpringApplication.run(NotificationApplication.class, args);
    }
}
