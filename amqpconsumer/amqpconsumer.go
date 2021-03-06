/*
Copyright (c) 2014-2015 Antonin Amand <antonin.amand@gmail.com>, All rights reserved.
See LICENSE file or http://www.opensource.org/licenses/BSD-3-Clause.
*/

/*
	Package amqpconsumer implements a Subscriber that pulls messages from AMQP.
*/
package amqpconsumer

import (
	"fmt"
	"github.com/streadway/amqp"
	"golang.org/x/net/context"

	"github.com/efimbakulin/celery"
	"github.com/efimbakulin/celery/amqputil"
	"github.com/efimbakulin/celery/logging"
)

type amqpTask struct {
	context.Context
	msg *celery.Message
	ch  *amqp.Channel
	tag uint64 // delivery tag
}

type Config struct {
	Exchange string

	// queue
	QDurable    bool
	QAutoDelete bool
	QExclusive  bool
	QNoWait     bool
	QArgs       amqp.Table // queue extra arguments

	// consumer
	Consumer   string // consumer name
	CAutoACK   bool
	CExclusive bool
	CNoLocal   bool
	CNoWait    bool
	CArgs      amqp.Table // consumer extra arguments
}

var defaultConfig = &Config{
	Exchange: "celery",

	QDurable:    true,
	QAutoDelete: false,
	QExclusive:  false,
	QNoWait:     false,
	QArgs:       nil,

	Consumer:   "",
	CAutoACK:   false,
	CExclusive: false,
	CNoLocal:   false,
	CNoWait:    false,
	CArgs:      nil,
}

// DefaultConfig returns a config with the following defaults:
//
// 		QDurable:    false,
// 		QAutoDelete: false,
// 		QExclusive:  false,
// 		QNoWait:     false,
// 		QArgs:       nil,
// 		Consumer:    "",
// 		CAutoACK:    false,
// 		CExclusive:  false,
// 		CNoLocal:    false,
// 		CNoWait:     false,
// 		CArgs:       nil,
func DefaultConfig() Config {
	return *defaultConfig
}

func (t *amqpTask) Msg() celery.Message {
	return *t.msg
}

func (t *amqpTask) Ack() error {
	// XXX: test what happens with AutoAck
	return t.ch.Ack(t.tag, false)
}

func (t *amqpTask) Reject(requeue bool) error {
	// XXX: test what happens with AutoAck
	return t.ch.Reject(t.tag, requeue)
}

type amqpConsumer struct {
	q      string
	config *Config
	retry  *amqputil.Retry
	out    chan celery.Task
	quit   chan struct{}
	log    logging.Logger
}

var _ celery.Subscriber = (*amqpConsumer)(nil)

// NewAMQPSubscriber creates a new AMQP Subscriber. config can be nil, in
// which case it will be set with DefaultConfig.
func NewAMQPSubscriber(queue string, config *Config, retry *amqputil.Retry, log logging.Logger) celery.Subscriber {
	if config == nil {
		dcfg := DefaultConfig()
		config = &dcfg
	}
	c := &amqpConsumer{
		q:      queue,
		config: config,
		retry:  retry,
		out:    make(chan celery.Task),
		quit:   make(chan struct{}),
		log:    log,
	}
	go c.loop()
	return c
}

// Subscribe implements the Subscriber interface.
func (c *amqpConsumer) Subscribe() <-chan celery.Task {
	return c.out
}

// Close implements the Subscriber interface.
func (c *amqpConsumer) Close() error {
	close(c.quit)
	return nil
}

func (c *amqpConsumer) declare(ch *amqp.Channel) (<-chan amqp.Delivery, error) {
	if err := ch.ExchangeDeclare(
		c.config.Exchange,
		"direct",
		c.config.QDurable,     // durable
		c.config.QAutoDelete,  // auto-deleted
		false,                 // internal
		c.config.QNoWait,      // noWait
		nil,                   // arguments
	); err != nil {
		return nil, fmt.Errorf("Exchange declare error: %s", err)
	}
	q, err := ch.QueueDeclare(
		c.q,                  // name
		c.config.QDurable,    // durable
		c.config.QAutoDelete, // delete when usused
		c.config.QExclusive,  // exclusive
		c.config.QNoWait,     // no-wait
		c.config.QArgs,       // arguments
	)
	if err != nil {
		return nil, err
	}

	if err := ch.QueueBind(c.q, c.q, c.config.Exchange, false, nil); err != nil {
		return nil, err
	}

	msgs, err := ch.Consume(
		q.Name,              // queue
		c.config.Consumer,   // consumer
		c.config.CAutoACK,   // auto-ack
		c.config.CExclusive, // exclusive
		c.config.CNoLocal,   // no-local
		c.config.CNoWait,    // no-wait
		c.config.CArgs,      // args
	)
	if err != nil {
		return nil, err
	}

	return msgs, nil
}

func (c *amqpConsumer) rootContext() (context.Context, context.CancelFunc) {
	if c.config.CAutoACK {
		return context.Background(), func() {}
	}
	return context.WithCancel(context.Background())
}

func (c *amqpConsumer) loop() {

	var ch *amqp.Channel
	var task celery.Task
	var out chan celery.Task
	var msgs <-chan amqp.Delivery
	var in <-chan amqp.Delivery
	var ok bool

	ctx, abort := c.rootContext()
	chch := c.retry.Channel()

	defer close(c.out)
	defer func() {
		if ch != nil {
			ch.Close()
		}
	}()

	for {
		select { // carefull, order matters
		case <-c.quit: // quit
			abort()
			return
		case ch, ok = <-chch: // wait for an AMQP channel
			if !ok {
				c.log.Info("Terminated amqp consumer.")
				return
			}
			var err error
			msgs, err = c.declare(ch)
			if err != nil {
				if err != amqp.ErrClosed {
					panic(err)
				}
				chch = c.retry.Channel()
				continue
			}
			c.log.Info("New channel.")
			ctx, abort = c.rootContext()
			chch = nil
			in = msgs
		case out <- task: // send task downstream.
			out = nil
			in = msgs
		case d, ok := <-in: // wait for AMQP deliveries
			in = nil
			if !ok {
				c.log.Info("Closed messages")
				abort()
				msgs = nil
				out = nil
				chch = c.retry.Channel()
				continue
			}
			// log.Infof("%s %s", d.Body, d.ReplyTo)
			msg, err := celery.DecodeMessage(d.ContentType, d.Body)
			if err != nil {
				c.log.Error(err)
				d.Reject(true)
				continue
			}
			mctx := celery.ContextFromMessage(ctx, msg)
			task = &amqpTask{mctx, &msg, ch, d.DeliveryTag}
			out = c.out
		}
	}

}
