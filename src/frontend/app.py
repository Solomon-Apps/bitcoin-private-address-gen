from flask import Flask, render_template, jsonify, request
import pika
import json
import os
import threading
import queue

app = Flask(__name__)
results_queue = queue.Queue()

def get_rabbitmq_channel():
    rabbitmq_url = os.getenv('RABBITMQ_URL')
    connection = pika.BlockingConnection(pika.URLParameters(rabbitmq_url))
    return connection, connection.channel()

def rabbitmq_consumer():
    rabbitmq_url = os.getenv('RABBITMQ_URL')
    connection = pika.BlockingConnection(pika.URLParameters(rabbitmq_url))
    channel = connection.channel()

    channel.queue_declare(queue='bitcoin-addresss-gen-found', durable=True)

    def callback(ch, method, properties, body):
        results_queue.put(body.decode())

    channel.basic_consume(queue='bitcoin-addresss-gen-found',
                         on_message_callback=callback,
                         auto_ack=True)

    channel.start_consuming()

@app.route('/')
def index():
    return render_template('index.html')

@app.route('/api/results')
def get_results():
    results = []
    while not results_queue.empty():
        results.append(results_queue.get())
    return jsonify(results)

@app.route('/api/purge-queue', methods=['POST'])
def purge_queue():
    try:
        connection, channel = get_rabbitmq_channel()
        channel.queue_purge('bitcoin-address-gen')
        connection.close()
        return jsonify({"status": "success"})
    except Exception as e:
        return jsonify({"status": "error", "message": str(e)}), 500

if __name__ == '__main__':
    # Start RabbitMQ consumer in a separate thread
    consumer_thread = threading.Thread(target=rabbitmq_consumer)
    consumer_thread.daemon = True
    consumer_thread.start()

    app.run(host='0.0.0.0', port=5000) 