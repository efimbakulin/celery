/*
Copyright (c) 2015 Antonin Amand <antonin.amand@gmail.com>, All rights reserved.
See LICENSE file or http://www.opensource.org/licenses/BSD-3-Clause.
*/

// Package server provides utilities to run celery workers.
package server

import (
	"os"
	"os/signal"
	"time"

	"github.com/efimbakulin/celery"
	"github.com/efimbakulin/celery/amqpbackend"
	"github.com/efimbakulin/celery/amqpconsumer"
	"github.com/efimbakulin/celery/amqputil"
	"github.com/efimbakulin/celery/logging"

	_ "github.com/efimbakulin/celery/jsonmessage"
)

// Serve loads config from environment and runs a worker with an AMQP consumer and result backend.
// declare should be used to register tasks.
func Serve(queue string, declare func(worker *celery.Worker), log logging.Logger, consumerConfig *amqpconsumer.Config, discardResults bool) {
	conf := celery.ConfigFromEnv()

	retry := amqputil.NewRetry(conf.BrokerURL, nil, 2*time.Second, log)
	sched := celery.NewScheduler(amqpconsumer.NewAMQPSubscriber(queue, consumerConfig, retry, log), log)
    var backend celery.Backend
    if discardResults {
        backend = &celery.DiscardBackend{}
    } else {
        backend = amqpbackend.NewAMQPBackend(retry, log)
    }

	worker := celery.NewWorker(conf.CelerydConcurrency, sched, backend, sched, log)

	quit := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill)

	go func() {
		s := <-sigs
		log.Infof("Signal %v received. Closing...", s)
		go func() {
			<-sigs
			os.Exit(1)
		}()
		worker.Close()
		worker.Wait()
		retry.Close()
		log.Info("Closed.")
		close(quit)
	}()

	declare(worker)
	worker.Start()

	<-quit

}
