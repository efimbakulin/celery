#! /usr/bin/env python


from celery import Celery

app = Celery('tasks', broker='amqp://guest@localhost//')

app.conf.update(
    BROKER_URL = 'amqp://guest:guest@localhost:5672//',
    CELERY_TASK_SERIALIZER='json',
)

@app.task()
def add(x, y):
    """ Golang task
    """
    pass

@app.task()
def two(d, key="value"):
    """ Golang task
    """
    pass

@app.task()
def unknown():
    pass

if __name__ == "__main__":
    for i in xrange(200):
        unknown.delay()

    two.apply_async(args=["LAST"], countdown=10)
    two.apply_async(args=["LATER"], countdown=2)

    for i in xrange(10):
        two.apply_async(args=["BULK"], countdown=10)

    add.delay(2, 3)
    two.delay(2)

    unknown.delay()

    two.delay(None, key="foo")
    two.apply_async(args=["LATER"], countdown=2)

    unknown.delay()