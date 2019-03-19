pipeline {
  agent {
    label 'master'
  }
  options {
    buildDiscarder(logRotator(numToKeepStr: '10'))
    disableConcurrentBuilds()
  }
  triggers {
    /* No polling: build is triggered by webhook */
    pollSCM('')
  }
  stages {
    stage('Go Report') {
      agent { label 'jenkins-slave-go' }
      steps {
        script {
          def result = sh(script: "goreportcard-cli -v", returnStdout: true).tokenize('\n')
          echo result.join("\n")
          if ( ! result.grep(~/^Grade: A.*/) ) {
            echo "Please enhance code to make it at least 'Grade: A'"
            echo "Try correcting format with: gofmt -w -s . "
            // error 'GoReportCard Grade is less than A'
            currentBuild.result = 'UNSTABLE'
          }
        }
      }
    }
    stage('Build') {
      steps {
        dir ('k8s-aws-accelerator-devplugin') {
          sh "./build.sh"
        }
        dir ('k8s-intel-accelerator-devplugin') {
          sh "./build.sh"
        }
      }
    }
  }
}
